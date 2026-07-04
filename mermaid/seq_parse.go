package mermaid

import (
	"regexp"
	"strings"
)

var (
	seqPartRe = regexp.MustCompile(`^(participant|actor)\s+([A-Za-z0-9_.-]+)(?:\s+as\s+(.+))?$`)
	// ponytail: the id class overlaps the arrow tokens, so an id containing a
	// literal arrow substring (`req-x-resp`) can split ambiguously — `a-x-b: t`
	// reads as `a -x -b`, same as mermaid's own grammar. A real tokenizer is
	// the upgrade path if that ever bites.
	seqMsgRe   = regexp.MustCompile(`^([A-Za-z0-9_.-]+?)\s*(-->>|--x|-->|->>|-x|->)\s*([+-]?)\s*([A-Za-z0-9_.-]+)\s*:\s*(.*)$`)
	seqNoteRe  = regexp.MustCompile(`(?i)^note\s+(left of|right of|over)\s+([A-Za-z0-9_.-]+)(?:\s*,\s*([A-Za-z0-9_.-]+))?\s*:\s*(.*)$`)
	seqActRe   = regexp.MustCompile(`^(activate|deactivate)\s+([A-Za-z0-9_.-]+)$`)
	seqFrameRe = regexp.MustCompile(`^(loop|opt|alt|par)(?:\s+(.*))?$`)
	seqDivRe   = regexp.MustCompile(`^(else|and)(?:\s+(.*))?$`)
)

func parseSequence(src string) (*SeqDiagram, error) {
	d := &SeqDiagram{}
	var stack []*SeqFrame

	curItems := func() *[]SeqItem {
		if len(stack) == 0 {
			return &d.Items
		}
		f := stack[len(stack)-1]
		return &f.Sections[len(f.Sections)-1].Items
	}
	ensure := func(id, label string, actor, explicit bool) error {
		if p := d.participant(id); p != nil {
			if explicit {
				return unsup("participant %q redeclared", id)
			}
			return nil
		}
		if label == "" {
			label = id
		}
		d.Participants = append(d.Participants, &Participant{ID: id, Label: label, Actor: actor})
		return nil
	}

	seenHeader := false
	for _, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if !seenHeader {
			if line != "sequenceDiagram" {
				return nil, unsup("bad header %q", line)
			}
			seenHeader = true
			continue
		}

		switch {
		case line == "autonumber":
			d.Autonumber = true

		case strings.HasPrefix(line, "autonumber "):
			return nil, unsup("autonumber arguments %q", line)

		case seqPartRe.MatchString(line):
			m := seqPartRe.FindStringSubmatch(line)
			if err := ensure(m[2], strings.TrimSpace(m[3]), m[1] == "actor", true); err != nil {
				return nil, err
			}

		case seqMsgRe.MatchString(line):
			m := seqMsgRe.FindStringSubmatch(line)
			if err := ensure(m[1], "", false, false); err != nil {
				return nil, err
			}
			if err := ensure(m[4], "", false, false); err != nil {
				return nil, err
			}
			msg := &SeqMessage{From: m[1], To: m[4], Text: strings.TrimSpace(m[5])}
			msg.Dashed = strings.HasPrefix(m[2], "--")
			// Head from the operator, normalized: ">>" -> arrow, "x" -> cross, else none.
			op := m[2]
			switch {
			case strings.HasSuffix(op, ">>"):
				msg.Head = HeadArrow
			case strings.HasSuffix(op, "x"):
				msg.Head = HeadCross
			default:
				msg.Head = HeadNone
			}
			switch m[3] {
			case "+":
				msg.ActivateTo = true
			case "-":
				msg.DeactivateFrom = true
			}
			*curItems() = append(*curItems(), msg)

		case seqNoteRe.MatchString(line):
			m := seqNoteRe.FindStringSubmatch(line)
			n := &SeqNote{A: m[2], B: m[3], Text: strings.TrimSpace(m[4])}
			switch strings.ToLower(m[1]) {
			case "left of":
				n.Pos = NoteLeft
			case "right of":
				n.Pos = NoteRight
			default:
				n.Pos = NoteOver
			}
			if n.Pos != NoteOver && n.B != "" {
				return nil, unsup("two participants only valid with 'over' %q", line)
			}
			*curItems() = append(*curItems(), n)

		case seqActRe.MatchString(line):
			m := seqActRe.FindStringSubmatch(line)
			*curItems() = append(*curItems(), &SeqActivate{P: m[2], On: m[1] == "activate"})

		case seqFrameRe.MatchString(line):
			m := seqFrameRe.FindStringSubmatch(line)
			f := &SeqFrame{Kind: m[1], Sections: []SeqSection{{Label: strings.TrimSpace(m[2])}}}
			stack = append(stack, f)

		case seqDivRe.MatchString(line):
			m := seqDivRe.FindStringSubmatch(line)
			if len(stack) == 0 {
				return nil, unsup("%q outside a frame", m[1])
			}
			f := stack[len(stack)-1]
			if (m[1] == "else" && f.Kind != "alt") || (m[1] == "and" && f.Kind != "par") {
				return nil, unsup("%q divider in %q frame", m[1], f.Kind)
			}
			f.Sections = append(f.Sections, SeqSection{Label: strings.TrimSpace(m[2])})

		case line == "end":
			if len(stack) == 0 {
				return nil, unsup("end without frame")
			}
			f := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			*curItems() = append(*curItems(), f)

		default:
			return nil, unsup("statement %q", line)
		}
	}
	if !seenHeader {
		return nil, unsup("empty diagram")
	}
	if len(stack) != 0 {
		return nil, unsup("unclosed %q frame", stack[len(stack)-1].Kind)
	}
	// Notes/activate/deactivate don't implicitly create participants, but may
	// forward-reference one declared later in the document (e.g. by a
	// message); validate against the fully-built participant set only now.
	var checkRefs func(items []SeqItem) error
	checkRefs = func(items []SeqItem) error {
		for _, it := range items {
			switch v := it.(type) {
			case *SeqNote:
				if d.participant(v.A) == nil || (v.B != "" && d.participant(v.B) == nil) {
					return unsup("note references unknown participant %q", v.Text)
				}
			case *SeqActivate:
				if d.participant(v.P) == nil {
					verb := "deactivate"
					if v.On {
						verb = "activate"
					}
					return unsup("%s of unknown participant %q", verb, v.P)
				}
			case *SeqFrame:
				for _, s := range v.Sections {
					if err := checkRefs(s.Items); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if err := checkRefs(d.Items); err != nil {
		return nil, err
	}
	return d, nil
}
