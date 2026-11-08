package runs

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/liamphmurphy/vesper/server/internal/agentdef"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Workflow images: a step with Images set generates pictures on Codex, and
// they are stored against that step's attempt. A step that makes none passes
// on what it received, so Generate -> Verify -> Gate -> Chat delivers
// Generate's image even though Verify only writes text.

const (
	// MaxWorkflowImagesPerStep bounds the image tool calls and the stored
	// images of one attempt.
	MaxWorkflowImagesPerStep = 4
	// maxWorkflowInputImages bounds what one step is shown from earlier steps.
	maxWorkflowInputImages = 8
	// workflowCodexLookup bounds model discovery, which runs under the chat's lock.
	workflowCodexLookup = 20 * time.Second
)

const workflowImageInstructions = `

This step generates images. Use your image generation tool, make at most %d, and copy each final file into .lazyagents/outbox/ with cp (png, jpeg, webp or gif). You can't change anything else. An image step that leaves no image in the outbox fails, whatever you submit.`

// WorkflowImage is one stored image and the attempt that made it.
type WorkflowImage = store.ListWorkflowRunImagesRow

// WorkflowImagesByNode says which images each node's newest attempt carries:
// its own, else everything its parents carry. Only newest attempts count, so
// a retry never sees an earlier attempt's images, and a stale node carries nothing.
func WorkflowImagesByNode(g WorkflowGraph, rows []store.LatestWorkflowStepsRow, all []WorkflowImage) map[string][]WorkflowImage {
	out := make(map[string][]WorkflowImage, len(g.Nodes))
	order, ok := workflowOrder(g)
	if !ok || len(all) == 0 {
		return out
	}
	parents := workflowParents(g)
	for _, n := range order {
		at := slices.IndexFunc(rows, func(r store.LatestWorkflowStepsRow) bool { return r.NodeID == n.ID })
		if at < 0 || rows[at].State == "stale" || rows[at].State == "failed" {
			continue
		}
		own := slices.DeleteFunc(slices.Clone(all), func(i WorkflowImage) bool { return i.StepID != rows[at].ID })
		if len(own) == 0 {
			own = parentImages(parents[n.ID], out)
		}
		out[n.ID] = own
	}
	return out
}

// parentImages joins what parents carry; two branches can carry the same image.
func parentImages(parents []string, carried map[string][]WorkflowImage) []WorkflowImage {
	var out []WorkflowImage
	seen := map[uuid.UUID]bool{}
	for _, p := range parents {
		for _, i := range carried[p] {
			if !seen[i.ID] {
				seen[i.ID] = true
				out = append(out, i)
			}
		}
	}
	return out
}

// workflowInputImages is what a node receives from the steps before it.
func workflowInputImages(g WorkflowGraph, node string, rows []store.LatestWorkflowStepsRow, all []WorkflowImage) []WorkflowImage {
	in := parentImages(workflowParents(g)[node], WorkflowImagesByNode(g, rows, all))
	return in[:min(len(in), maxWorkflowInputImages)]
}

// WorkflowStepInputImages is the images to show the running step agentRun.
func WorkflowStepInputImages(ctx context.Context, q *store.Queries, agentRun uuid.UUID) ([]WorkflowImage, error) {
	run, err := q.WorkflowRunForAgentRun(ctx, &agentRun)
	if err != nil {
		return nil, err
	}
	step, err := q.WorkflowStepForAgentRun(ctx, &agentRun)
	if err != nil {
		return nil, err
	}
	g, err := ParseWorkflowGraph(run.Graph)
	if err != nil {
		return nil, err
	}
	rows, err := q.LatestWorkflowSteps(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	all, err := q.ListWorkflowRunImages(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	return workflowInputImages(g, step.NodeID, rows, all), nil
}

// workflowStepWantedImage reports a finished step that was asked for an image
// and has none. The runner fails such a step first, with Codex's reason; this
// keeps a step that ended any other way from passing on text alone.
func workflowStepWantedImage(ctx context.Context, q *store.Queries, g WorkflowGraph, r store.LatestWorkflowStepsRow) (bool, error) {
	if !slices.ContainsFunc(g.Nodes, func(n WorkflowNode) bool { return n.ID == r.NodeID && n.Type == NodeAgent && n.Config.Images }) {
		return false, nil
	}
	n, err := q.CountWorkflowStepImages(ctx, r.ID)
	return n == 0, err
}

// workflowImagePersona checks that the persona a step runs as may make images:
// the node's persona, else the persona answering the chat. A non-empty result
// is the reason the step can't run.
func workflowImagePersona(ctx context.Context, q *store.Queries, chat store.ChatSession, n WorkflowNode) (string, error) {
	const pick = "Image steps need a persona that can generate images: pick one for this step, or run it from a chat with one."
	id := ChatSpeakerID(chat)
	if n.Config.PersonaID != "" {
		parsed, err := uuid.Parse(n.Config.PersonaID)
		if err != nil {
			return "This step's persona no longer exists.", nil
		}
		id = &parsed
	}
	if id == nil {
		return pick, nil
	}
	p, err := q.GetPersona(ctx, store.GetPersonaParams{ID: *id, WorkspaceID: chat.WorkspaceID, UserID: chat.CreatedBy})
	if errors.Is(err, pgx.ErrNoRows) {
		if n.Config.PersonaID != "" {
			return "This step's persona no longer exists.", nil
		}
		return pick, nil
	} else if err != nil {
		return "", err
	}
	if !p.CanGenerateImages {
		return p.Name + " can't generate images. Turn it on in Manage personas, or pick another persona for this step.", nil
	}
	return "", nil
}

// workflowCodexModel picks the Codex model for an image step: the chat's own
// on a Codex chat, else Codex's default. A non-empty reason is why it can't run.
func (e *WorkflowEngine) workflowCodexModel(ctx context.Context, q *store.Queries, chat store.ChatSession) (model, reason string, err error) {
	if chat.Harness == agentdef.HarnessCodex && chat.Model != "" {
		return chat.Model, "", nil
	}
	ctx, cancel := context.WithTimeout(ctx, workflowCodexLookup)
	defer cancel()
	catalog, err := e.Catalog.List(ctx, q, chat.WorkspaceID)
	if err != nil {
		return "", "", err
	}
	why := "Codex isn't available"
	for _, h := range catalog {
		if h.Harness != agentdef.HarnessCodex {
			continue
		}
		if !h.Available {
			why = h.UnavailableReason
			break
		}
		for _, m := range h.Models {
			if model == "" || m.Default {
				model = m.ID
			}
		}
		if model == "" {
			why = "no Codex models are available"
		}
	}
	if model == "" {
		return "", "Image steps run on Codex: " + why + ". Set up Codex in Settings → Connections & secrets, then run this again.", nil
	}
	return model, "", nil
}

var workflowImageLinkRe = regexp.MustCompile(`!\[[^\]]*\]\(([^)]*)\)`)

// stripWorkflowImageLinks drops markdown images that point at local files. A
// step's images are delivered by Vesper, so a link to one would only break.
func stripWorkflowImageLinks(body string) string {
	return strings.TrimSpace(workflowImageLinkRe.ReplaceAllStringFunc(body, func(m string) string {
		target := strings.ToLower(strings.TrimSpace(workflowImageLinkRe.FindStringSubmatch(m)[1]))
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			return m
		}
		return ""
	}))
}

// workflowImageTitle numbers the Library items of a step that made several.
func workflowImageTitle(title string, i, total int) string {
	if total < 2 {
		return title
	}
	return fmt.Sprintf("%s (%d)", title, i+1)
}
