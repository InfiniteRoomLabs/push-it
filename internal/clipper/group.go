// Package clipper turns a word-level transcript into reviewed sound clips.
package clipper

import "strings"

// Word is one transcribed word with timestamps in seconds.
type Word struct {
	Word  string  `json:"word"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// Phrase is a candidate clip.
type Phrase struct {
	ID    int     `json:"id"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Label string  `json:"label"`
	File  string  `json:"file,omitempty"`
}

// Options control grouping.
type Options struct {
	Phrase []string // the words a phrase must start with, in order
	Allow  []string // extra words allowed inside a phrase
	Gap    float64  // max silence (s) between consecutive words
	Max    float64  // max phrase length (s)
}

// DefaultOptions matches the default "push it" phrase.
func DefaultOptions() Options {
	return Options{Phrase: []string{"push", "it"}, Allow: []string{"real", "good"}, Gap: 0.5, Max: 4.0}
}

func clean(s string) string { return strings.ToLower(strings.Trim(s, ".,!?\"' ")) }

func (o Options) allowed() map[string]bool {
	m := map[string]bool{}
	for _, w := range append(append([]string{}, o.Phrase...), o.Allow...) {
		m[clean(w)] = true
	}
	return m
}

// startsPhrase reports whether words[i:] begins with o.Phrase within Gap.
func (o Options) startsPhrase(words []Word, i int) bool {
	if i+len(o.Phrase) > len(words) {
		return false
	}
	for k, want := range o.Phrase {
		if clean(words[i+k].Word) != clean(want) {
			return false
		}
		if k > 0 && words[i+k].Start-words[i+k-1].End > o.Gap {
			return false
		}
	}
	return true
}

// Group finds phrases: runs of allowed words that begin with o.Phrase,
// separated by no more than Gap seconds, capped at Max seconds.
func Group(words []Word, o Options) []Phrase {
	allowed := o.allowed()
	var out []Phrase
	i := 0
	for i < len(words) {
		if !o.startsPhrase(words, i) {
			i++
			continue
		}
		start := words[i].Start
		j := i + len(o.Phrase)
		for j < len(words) {
			wd := words[j]
			if wd.Start-words[j-1].End > o.Gap || !allowed[clean(wd.Word)] || wd.End-start > o.Max {
				break
			}
			j++
		}
		labels := make([]string, 0, j-i)
		for k := i; k < j; k++ {
			labels = append(labels, clean(words[k].Word))
		}
		out = append(out, Phrase{ID: len(out) + 1, Start: start, End: words[j-1].End, Label: strings.Join(labels, " ")})
		i = j
	}
	return out
}
