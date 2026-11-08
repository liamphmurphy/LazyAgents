package harness

import "strings"

// richReplyBlocks are the fenced blocks the chat renders as something other
// than code. They can take a while to write and call no tool.
var richReplyBlocks = map[string]bool{"app": true, "svg-image": true, "mermaid": true, "choices": true, "workflow-run": true, "recommend": true}

// replyScanner finds rich blocks in a reply as it streams, so the chat can say
// what's being written before the finished message arrives. A block is seen once
// its opening line is complete. Fences inside another code block don't count.
type replyScanner struct {
	line    string
	inFence bool
}

func (s *replyScanner) feed(text string) []Event {
	var out []Event
	s.line += text
	for {
		i := strings.IndexByte(s.line, '\n')
		if i < 0 {
			return out
		}
		line := strings.TrimSpace(s.line[:i])
		s.line = s.line[i+1:]
		if s.inFence {
			s.inFence = line != "```"
			continue
		}
		if kind, ok := strings.CutPrefix(line, "```"); ok {
			s.inFence = true
			if richReplyBlocks[kind] {
				out = append(out, Event{"lifecycle", map[string]any{"state": "reply_block", "kind": kind}})
			}
		}
	}
}
