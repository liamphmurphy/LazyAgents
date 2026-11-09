package usersecrets

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/liamphmurphy/vesper/server/internal/store"
)

const (
	sendTimeout  = 30 * time.Second
	maxBody      = 1 << 20
	maxResponse  = 1 << 20
	maxRedirects = 5
)

// Request is what a run asks to send. The secret isn't in it anywhere.
type Request struct {
	Method      string
	URL         string
	Headers     map[string]string
	Body        []byte
	ContentType string
}

// Response is what the run gets back, with every copy of the value masked.
type Response struct {
	Status      int               `json:"status"`
	Headers     map[string]string `json:"headers"`
	Body        string            `json:"body"`
	Truncated   bool              `json:"truncated,omitempty"`
	Redactions  int               `json:"redactions,omitempty"`
	ContentType string            `json:"content_type,omitempty"`
}

// Blocked is a request refused before anything was sent. Its text is for the
// run (and the person) and never contains the value.
type Blocked struct{ Reason string }

func (b *Blocked) Error() string { return b.Reason }

func blocked(format string, args ...any) error { return &Blocked{Reason: fmt.Sprintf(format, args...)} }

// Checked is a request that passed every check that doesn't need the person.
type Checked struct {
	Method string
	URL    *url.URL
	Write  bool
	Policy string
}

var methods = []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}

// agentHeaders are headers a run may never set: they'd carry or replace a
// credential, or confuse the request framing.
var agentHeaders = []string{"authorization", "proxy-authorization", "cookie", "host", "content-length", "transfer-encoding", "connection", "te", "upgrade"}

// isLocal is a host on this machine; plain http and private addresses are
// allowed only for these, as for connectors.
func isLocal(host string) bool {
	ip := net.ParseIP(host)
	return host == "localhost" || host == "host.docker.internal" || (ip != nil && ip.IsLoopback())
}

// Check applies the secret's own rules to a request: not expired, a method
// it allows, https to an allowed host, and no headers that would carry or
// replace a credential. Ask is left to the caller.
func (s *Service) Check(sec store.UserSecret, req Request) (Checked, error) {
	var c Checked
	if sec.ExpiresAt != nil && !s.now().Before(*sec.ExpiresAt) {
		return c, blocked("%s has expired. Ask the person to replace it in Customization › Secrets.", sec.Handle)
	}
	c.Method = strings.ToUpper(strings.TrimSpace(req.Method))
	if c.Method == "" {
		c.Method = http.MethodGet
	}
	if !slices.Contains(methods, c.Method) {
		return c, blocked("method must be one of %s", strings.Join(methods, ", "))
	}
	c.Write = c.Method != http.MethodGet && c.Method != http.MethodHead
	c.Policy = sec.ReadPolicy
	if c.Write {
		c.Policy = sec.WritePolicy
	}
	if c.Policy == PolicyOff {
		kind := "read"
		if c.Write {
			kind = "write"
		}
		return c, blocked("%s requests with %s are turned off.", kind, sec.Handle)
	}
	u, err := url.Parse(strings.TrimSpace(req.URL))
	if err != nil || u.Host == "" || len(req.URL) > 4000 {
		return c, blocked("url must be a full URL, e.g. https://%s/path", sec.AllowedHosts[0])
	}
	if u.User != nil {
		return c, blocked("url can't contain credentials")
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if !slices.Contains(sec.AllowedHosts, host) {
		return c, blocked("%s can only be sent to %s.", sec.Handle, strings.Join(sec.AllowedHosts, ", "))
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isLocal(host)) {
		return c, blocked("use https (http only works for localhost)")
	}
	if sec.Placement == PlacementQuery && u.Query().Has(sec.PlacementKey) {
		return c, blocked("leave the %s query parameter out: Vesper adds it", sec.PlacementKey)
	}
	for k := range req.Headers {
		lk := strings.ToLower(k)
		if slices.Contains(agentHeaders, lk) || (sec.Placement == PlacementHeader && strings.EqualFold(k, sec.PlacementKey)) {
			return c, blocked("the %s header can't be set: Vesper adds the secret itself", k)
		}
		if strings.ContainsAny(k+req.Headers[k], "\r\n") {
			return c, blocked("header %q has a line break", k)
		}
	}
	if len(req.Body) > maxBody {
		return c, blocked("the body is limited to 1 MB")
	}
	if len(req.Body) > 0 && !c.Write {
		return c, blocked("%s requests can't have a body", c.Method)
	}
	u.Host = strings.ToLower(u.Host)
	c.URL = u
	return c, nil
}

// blockedAddr is an address a request may not reach unless its host is local:
// private networks, link-local (cloud metadata) and the like.
func blockedAddr(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() ||
		ip.IsMulticast() || ip.IsInterfaceLocalMulticast() || (ip.To4() != nil && ip.To4()[0] == 0) ||
		// 100.64.0.0/10, carrier-grade NAT, often internal
		(ip.To4() != nil && ip.To4()[0] == 100 && ip.To4()[1]&0xc0 == 64)
}

// transport dials only addresses it checked, so a hostname can't resolve to
// a public address for the check and a private one for the connection.
func (s *Service) transport(host string) *http.Transport {
	lookup := s.LookupIP
	if lookup == nil {
		lookup = func(ctx context.Context, h string) ([]net.IP, error) {
			return net.DefaultResolver.LookupIP(ctx, "ip", h)
		}
	}
	local := isLocal(host)
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return &http.Transport{
		Proxy:                 nil, // An environment proxy would dial for us, past the address check.
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: sendTimeout,
		MaxIdleConns:          1,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			h, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips := []net.IP{net.ParseIP(h)}
			if ips[0] == nil {
				if ips, err = lookup(ctx, h); err != nil || len(ips) == 0 {
					return nil, fmt.Errorf("couldn't resolve %s", h)
				}
			}
			for _, ip := range ips {
				if blockedAddr(ip) && !local {
					return nil, &Blocked{Reason: h + " resolves to a private address, which secrets can't be sent to"}
				}
			}
			// Every address was checked above; try each, as net.Dial would.
			var last error
			for _, ip := range ips {
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return conn, nil
				}
				last = err
			}
			return nil, last
		},
	}
}

// inject adds the secret to an outgoing request, exactly where the person said.
func inject(r *http.Request, sec store.UserSecret, value string) {
	switch sec.Placement {
	case PlacementBearer:
		r.Header.Set("Authorization", "Bearer "+value)
	case PlacementHeader:
		r.Header.Set(sec.PlacementKey, value)
	case PlacementQuery:
		q := r.URL.Query()
		q.Set(sec.PlacementKey, value)
		r.URL.RawQuery = q.Encode()
	case PlacementBasic:
		user, pass, _ := strings.Cut(value, ":")
		r.SetBasicAuth(user, pass)
	}
}

// Send makes a checked request with the secret added. The value is masked as
// ‹handle› in the response body, its headers and any error text.
func (s *Service) Send(ctx context.Context, sec store.UserSecret, value string, c Checked, req Request) (Response, error) {
	mask := newMasker(sec, value)
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	var body io.Reader
	if len(req.Body) > 0 {
		body = strings.NewReader(string(req.Body))
	}
	out, err := http.NewRequestWithContext(ctx, c.Method, c.URL.String(), body)
	if err != nil {
		return Response{}, errors.New(mask.text(err.Error()))
	}
	for k, v := range req.Headers {
		out.Header.Set(k, v)
	}
	if len(req.Body) > 0 && req.ContentType != "" && out.Header.Get("Content-Type") == "" {
		out.Header.Set("Content-Type", req.ContentType)
	}
	out.Header.Set("User-Agent", "Vesper")
	inject(out, sec, value)
	host := c.URL.Hostname()
	t := s.transport(host)
	defer t.CloseIdleConnections()
	client := &http.Client{Transport: t, CheckRedirect: func(next *http.Request, via []*http.Request) error {
		// Only to the same host and scheme: anywhere else, the run gets the 3xx
		// and can decide what to do, and the secret stays where it was allowed.
		if len(via) >= maxRedirects || !strings.EqualFold(next.URL.Hostname(), host) || next.URL.Scheme != c.URL.Scheme {
			return http.ErrUseLastResponse
		}
		if sec.Placement == PlacementQuery {
			q := next.URL.Query()
			q.Set(sec.PlacementKey, value)
			next.URL.RawQuery = q.Encode()
		} else {
			inject(next, sec, value)
		}
		return nil
	}}
	res, err := client.Do(out)
	if err != nil {
		var b *Blocked
		if errors.As(err, &b) {
			return Response{}, b
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return Response{}, errors.New("the request timed out after 30 s")
		}
		return Response{}, errors.New(mask.text(err.Error()))
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponse+1))
	if err != nil {
		return Response{}, errors.New(mask.text("reading the response: " + err.Error()))
	}
	resp := Response{Status: res.StatusCode, Headers: map[string]string{}, ContentType: res.Header.Get("Content-Type")}
	if len(raw) > maxResponse {
		raw, resp.Truncated = raw[:maxResponse], true
	}
	for k, vs := range res.Header {
		if strings.EqualFold(k, "Set-Cookie") {
			continue
		}
		resp.Headers[k] = mask.text(strings.Join(vs, ", "))
	}
	if textual(resp.ContentType, raw) {
		resp.Body = mask.text(string(raw))
	} else {
		resp.Body = fmt.Sprintf("(%d bytes of %s, not shown)", len(raw), resp.ContentType)
	}
	resp.Redactions = mask.count
	return resp, nil
}

func textual(contentType string, raw []byte) bool {
	ct := strings.ToLower(contentType)
	for _, t := range []string{"text/", "json", "xml", "javascript", "x-www-form-urlencoded", "yaml", "csv"} {
		if strings.Contains(ct, t) {
			return true
		}
	}
	return ct == "" && !slices.Contains(raw, 0) && strings.ToValidUTF8(string(raw), "") == string(raw)
}

// masker replaces a value, and the forms it's commonly echoed in, with a
// placeholder, counting how many it replaced.
type masker struct {
	forms []string
	label string
	count int
}

func newMasker(sec store.UserSecret, value string) *masker {
	m := &masker{label: "‹" + sec.Handle + "›"}
	add := func(f string) {
		// Short fragments would mask ordinary text; a value this short can't be
		// told apart from the rest of a response anyway.
		if len(f) >= 8 && !slices.Contains(m.forms, f) {
			m.forms = append(m.forms, f)
		}
	}
	for _, v := range []string{value, strings.TrimSpace(value)} {
		add(v)
		add(url.QueryEscape(v))
		add(url.PathEscape(v))
		add(base64.StdEncoding.EncodeToString([]byte(v)))
		add(base64.RawStdEncoding.EncodeToString([]byte(v)))
		add(base64.URLEncoding.EncodeToString([]byte(v)))
		add(base64.RawURLEncoding.EncodeToString([]byte(v)))
	}
	if sec.Placement == PlacementBasic {
		if _, pass, ok := strings.Cut(value, ":"); ok {
			add(pass)
		}
	}
	// A JWT's header and claims aren't secret, but its signature is: mask it
	// even when it comes back on its own.
	if parts := strings.Split(strings.TrimSpace(value), "."); len(parts) == 3 && len(parts[2]) >= 16 {
		add(parts[2])
	}
	// Longest first, so a whole value is replaced before a part of it.
	slices.SortFunc(m.forms, func(a, b string) int { return len(b) - len(a) })
	return m
}

func (m *masker) text(s string) string {
	for _, f := range m.forms {
		if n := strings.Count(s, f); n > 0 {
			m.count += n
			s = strings.ReplaceAll(s, f, m.label)
		}
	}
	return s
}
