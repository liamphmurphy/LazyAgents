package playground

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// The probe is a small script the gateway puts into each page the Preview
// tab loads. It reports to Vesper's page, by postMessage and only to
// Vesper's origins, what the panel can't see across origins: where the app
// navigated, console errors, and requests that failed. And it follows the
// panel's back and forward buttons. It reads nothing else and changes
// nothing in the app.

// maxProbedPage bounds a page the gateway buffers to put the probe in;
// a larger one is passed through without it.
const maxProbedPage = 8 << 20

var headTag = regexp.MustCompile(`(?i)<head(\s[^>]*)?>`)

const probeTag = `<script src="` + ReservedPrefix + `/probe.js"></script>`

// injectProbe adds the probe to an HTML page the app sent uncompressed.
func injectProbe(res *http.Response) error {
	ct := strings.ToLower(res.Header.Get("Content-Type"))
	if !strings.HasPrefix(ct, "text/html") || res.Header.Get("Content-Encoding") != "" || res.ContentLength > maxProbedPage {
		return nil
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, maxProbedPage+1))
	res.Body.Close()
	if err != nil {
		return err
	}
	if len(b) <= maxProbedPage {
		if loc := headTag.FindIndex(b); loc != nil {
			b = append(b[:loc[1]:loc[1]], append([]byte(probeTag), b[loc[1]:]...)...)
		} else {
			b = append([]byte(probeTag), b...)
		}
	}
	res.Body = io.NopCloser(bytes.NewReader(b))
	res.ContentLength = int64(len(b))
	res.Header.Set("Content-Length", strconv.Itoa(len(b)))
	res.Header.Del("ETag") // the page isn't the one the app tagged
	return nil
}

func (g *Gateway) probe(w http.ResponseWriter) {
	origins, _ := json.Marshal(g.Origins)
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	fmt.Fprintf(w, probeScript, origins)
}

// probeScript is formatted with the JSON list of Vesper's origins.
const probeScript = `(() => {
  if (window.top === window || window.__vesperProbe) return;
  window.__vesperProbe = true;
  const origins = %s;
  const send = (m) => { for (const o of origins) { try { parent.postMessage(Object.assign({ vesperProbe: 1 }, m), o); } catch (_) {} } };
  const clip = (s) => { s = String(s); return s.length > 2000 ? s.slice(0, 2000) + "…" : s; };
  const fmt = (v) => {
    if (v instanceof Error) return clip(v.stack ? v.stack.split("\n").slice(0, 3).join("\n") : v.message);
    if (typeof v === "string") return clip(v);
    try { return clip(JSON.stringify(v)); } catch (_) { return clip(v); }
  };
  const where = () => send({ type: "location", path: location.pathname + location.search + location.hash, title: document.title });
  for (const m of ["pushState", "replaceState"]) {
    const orig = history[m];
    history[m] = function () { const r = orig.apply(this, arguments); where(); return r; };
  }
  addEventListener("popstate", where);
  addEventListener("hashchange", where);
  if (document.readyState === "loading") addEventListener("DOMContentLoaded", where); else where();
  const err = console.error;
  console.error = function () { send({ type: "console", text: Array.from(arguments).map(fmt).join(" ") }); return err.apply(this, arguments); };
  addEventListener("error", (e) => { if (e.message) send({ type: "console", text: clip(e.message + (e.filename ? " (" + e.filename + ":" + e.lineno + ")" : "")) }); });
  addEventListener("unhandledrejection", (e) => send({ type: "console", text: "Unhandled rejection: " + fmt(e.reason) }));
  const failed = (method, url, status, id, error) => send({ type: "request", method: String(method || "GET").toUpperCase(), url: clip(url), status, id: id || "", error: error || "" });
  const f = window.fetch;
  if (f) window.fetch = async function (input, init) {
    const method = (init && init.method) || (input && input.method) || "GET";
    const url = (input && input.url) || String(input);
    try {
      const r = await f.apply(this, arguments);
      if (r.status >= 400) failed(method, url, r.status, r.headers.get("X-Request-Id"));
      return r;
    } catch (e) { failed(method, url, 0, "", fmt(e)); throw e; }
  };
  const open = XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open = function (method, url) {
    this.addEventListener("loadend", () => { if (this.status === 0 || this.status >= 400) failed(method, url, this.status, this.getResponseHeader("X-Request-Id"), this.status === 0 ? "network error" : ""); });
    return open.apply(this, arguments);
  };
  addEventListener("message", (e) => {
    if (!origins.includes(e.origin) || !e.data || !e.data.vesperProbe) return;
    if (e.data.type === "back") history.back();
    if (e.data.type === "forward") history.forward();
  });
})();
`
