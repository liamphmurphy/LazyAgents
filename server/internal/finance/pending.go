package finance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/apperr"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// A pending import is a CSV whose layout Vesper doesn't know yet. The import
// dialog shows its first rows with a mapping to correct, the assistant
// proposes one in a hidden chat, and nothing is imported until the person
// confirms a mapping that reads every row. The mapping is then saved, so the
// next export from that bank imports directly.

// PendingTTL is how long an unfinished mapping step is kept.
const PendingTTL = 24 * time.Hour

// sampleRows is how many of the file's first rows the dialog and the
// assistant see: enough to tell a header from account details above it.
const sampleRows = 12

type Pending struct {
	ID        uuid.UUID
	AccountID uuid.UUID
	Filename  string
	ChatID    *uuid.UUID
	Sample    CSVSample
	Prefill   CSVMapping
	// Mapping is the assistant's, once it proposed one that reads the file.
	Mapping     *CSVMapping
	MappingNote string
	CreatedAt   time.Time
	// raw is the start of the file, for the assistant's prompt.
	raw []byte
}

// Preview is what a mapping reads from the whole file.
type Preview struct {
	Count     int
	FirstDate time.Time
	LastDate  time.Time
	InCents   int64
	OutCents  int64
	Rows      []Txn // the first few, in file order
	Balance   *Balance
}

func previewOf(p Parsed) Preview {
	out := Preview{Count: len(p.Txns), Balance: p.Balance, Rows: p.Txns[:min(len(p.Txns), 6)]}
	for _, t := range p.Txns {
		if out.FirstDate.IsZero() || t.PostedOn.Before(out.FirstDate) {
			out.FirstDate = t.PostedOn
		}
		if t.PostedOn.After(out.LastDate) {
			out.LastDate = t.PostedOn
		}
		if t.AmountCents > 0 {
			out.InCents += t.AmountCents
		} else {
			out.OutCents -= t.AmountCents
		}
	}
	return out
}

func (s *Service) savedMapping(ctx context.Context, o Owner, fingerprint string) (CSVMapping, bool, error) {
	var m CSVMapping
	raw, err := s.q().GetFinanceCSVMapping(ctx, store.GetFinanceCSVMappingParams{WorkspaceID: o.Workspace, UserID: o.User, Fingerprint: fingerprint})
	if errors.Is(err, pgx.ErrNoRows) {
		return m, false, nil
	}
	if err != nil {
		return m, false, err
	}
	// A mapping that no longer decodes is as good as none: map it again.
	if json.Unmarshal(raw, &m) != nil {
		return m, false, nil
	}
	return m, true, nil
}

func (s *Service) createPending(ctx context.Context, o Owner, account uuid.UUID, filename string, data []byte, prefill CSVMapping) (Pending, error) {
	q := s.q()
	if err := q.SweepFinancePendingImports(ctx, store.SweepFinancePendingImportsParams{WorkspaceID: o.Workspace, UserID: o.User, Before: time.Now().Add(-PendingTTL)}); err != nil {
		return Pending{}, err
	}
	raw, err := json.Marshal(prefill)
	if err != nil {
		return Pending{}, err
	}
	row, err := q.CreateFinancePendingImport(ctx, store.CreateFinancePendingImportParams{WorkspaceID: o.Workspace, UserID: o.User, AccountID: account,
		Filename: importName(filename), Data: data, Prefill: raw})
	if err != nil {
		return Pending{}, err
	}
	return pendingOf(row)
}

func pendingOf(row store.FinancePendingImport) (Pending, error) {
	out := Pending{ID: row.ID, AccountID: row.AccountID, Filename: row.Filename, ChatID: row.ChatID, MappingNote: row.MappingNote, CreatedAt: row.CreatedAt,
		raw: row.Data[:min(len(row.Data), 2048)]}
	if err := json.Unmarshal(row.Prefill, &out.Prefill); err != nil {
		return out, fmt.Errorf("pending import %s: prefill: %w", row.ID, err)
	}
	if row.Mapping != nil {
		var m CSVMapping
		if err := json.Unmarshal(row.Mapping, &m); err != nil {
			return out, fmt.Errorf("pending import %s: mapping: %w", row.ID, err)
		}
		out.Mapping = &m
	}
	sample, err := SampleCSV(row.Data, out.Prefill.Delimiter, sampleRows)
	out.Sample = sample
	return out, err
}

func (s *Service) pendingRow(ctx context.Context, o Owner, id uuid.UUID) (store.FinancePendingImport, error) {
	row, err := s.q().GetFinancePendingImport(ctx, store.GetFinancePendingImportParams{ID: id, WorkspaceID: o.Workspace, UserID: o.User})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, apperr.ErrNotFound
	}
	return row, err
}

func (s *Service) GetPending(ctx context.Context, o Owner, id uuid.UUID) (Pending, error) {
	row, err := s.pendingRow(ctx, o, id)
	if err != nil {
		return Pending{}, err
	}
	return pendingOf(row)
}

// AttachPendingChat records the hidden chat the assistant maps the file in.
func (s *Service) AttachPendingChat(ctx context.Context, id, chat uuid.UUID) error {
	return s.q().SetFinancePendingChat(ctx, store.SetFinancePendingChatParams{ID: id, ChatID: &chat})
}

// PreviewPending reads the whole file with a mapping, without importing.
func (s *Service) PreviewPending(ctx context.Context, o Owner, id uuid.UUID, m CSVMapping) (Preview, error) {
	row, err := s.pendingRow(ctx, o, id)
	if err != nil {
		return Preview{}, err
	}
	p, err := ParseCSVWith(row.Data, m)
	if err != nil {
		return Preview{}, err
	}
	return previewOf(p), nil
}

// ProposeMapping is the assistant's answer: kept only if it reads every row,
// otherwise the error says which row didn't, so it can try again.
func (s *Service) ProposeMapping(ctx context.Context, o Owner, id uuid.UUID, m CSVMapping, note string) (Preview, error) {
	row, err := s.pendingRow(ctx, o, id)
	if err != nil {
		return Preview{}, err
	}
	p, err := ParseCSVWith(row.Data, m)
	if err != nil {
		return Preview{}, err
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return Preview{}, err
	}
	note = strings.Join(strings.Fields(note), " ")
	if r := []rune(note); len(r) > 500 {
		note = string(r[:500])
	}
	return previewOf(p), s.q().SetFinancePendingMapping(ctx, store.SetFinancePendingMappingParams{ID: row.ID, Mapping: raw, MappingNote: note})
}

// CommitPending imports the file with the mapping the person confirmed and
// saves that mapping for the next file with the same layout.
func (s *Service) CommitPending(ctx context.Context, o Owner, id uuid.UUID, m CSVMapping) (ImportResult, error) {
	row, err := s.pendingRow(ctx, o, id)
	if err != nil {
		return ImportResult{}, err
	}
	p, err := ParseCSVWith(row.Data, m)
	if err != nil {
		return ImportResult{}, err
	}
	out, err := s.write(ctx, o, p, row.Filename, &row.AccountID)
	if err != nil {
		return out, err
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return out, err
	}
	q := s.q()
	if err := q.SaveFinanceCSVMapping(ctx, store.SaveFinanceCSVMappingParams{WorkspaceID: o.Workspace, UserID: o.User, Fingerprint: CSVFingerprint(row.Data), Mapping: raw}); err != nil {
		return out, err
	}
	_, err = q.DeleteFinancePendingImport(ctx, store.DeleteFinancePendingImportParams{ID: row.ID, WorkspaceID: o.Workspace, UserID: o.User})
	return out, err
}

func (s *Service) CancelPending(ctx context.Context, o Owner, id uuid.UUID) error {
	n, err := s.q().DeleteFinancePendingImport(ctx, store.DeleteFinancePendingImportParams{ID: id, WorkspaceID: o.Workspace, UserID: o.User})
	if err == nil && n == 0 {
		return apperr.ErrNotFound
	}
	return err
}

// MappingPrompt asks the assistant, in the pending import's hidden chat, for
// a mapping through finance_map_csv. It sees the file's first rows only.
func MappingPrompt(p Pending, accountKind string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The person is importing %q into their %s account in the Finance app, and Vesper doesn't know this CSV's layout. Work out which columns hold what, then call finance_map_csv with import_id %s and your mapping. It reads every row with it and tells you the first one that doesn't read: fix the mapping and call again until it's accepted. Then reply in one short sentence saying how you read the file. Don't call any other tool.\n\n", p.Filename, accountKind, p.ID)
	b.WriteString("Mapping rules: columns are 0-based. header_row is the 1-based row with the column names (0 if there is none); rows above it are skipped, and blank lines aren't rows. description lists 1–3 columns to join (a payee and a memo, say). Amounts are one of: a signed amount column; an unsigned amount column plus a direction column, with debit_values listing the direction values that mean money out; or separate debit (money out) and credit (money in) columns. ")
	fmt.Fprintf(&b, "date_format is one of %s. Set decimal_comma for amounts written 1.234,56. Set flip_sign when purchases show as positive numbers and payments as negative, which card exports often do.\n\n", strings.Join(dateFormatOrder, ", "))
	b.WriteString("Its first lines as they are in the file:\n```\n")
	lines := strings.SplitN(string(p.raw), "\n", 6)
	for _, line := range lines[:min(len(lines), 5)] {
		b.WriteString(strings.TrimRight(line, "\r") + "\n")
	}
	b.WriteString("```\n\n")
	delim := map[string]string{",": "comma", ";": "semicolon", "\t": "tab", "|": "pipe"}[p.Sample.Delimiter]
	fmt.Fprintf(&b, "The file's first %d rows, split on %s (row number: cells as JSON):\n", len(p.Sample.Rows), delim)
	for i, r := range p.Sample.Rows {
		raw, _ := json.Marshal(r)
		fmt.Fprintf(&b, "%d: %s\n", i+1, raw)
	}
	return b.String()
}
