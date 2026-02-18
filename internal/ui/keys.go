package ui

import (
	"github.com/basecamp/gliff/tui"
)

type KeyBinding struct {
	Keys []KeySpec
	Help KeyHelp
}

type KeyHelp struct {
	Key  string
	Desc string
}

type KeySpec struct {
	Type tui.KeyType
	Rune rune // only when Type == tui.KeyRune
}

func NewKeyBinding(specs ...KeySpec) KeyBinding {
	return KeyBinding{Keys: specs}
}

func Key(t tui.KeyType) KeySpec {
	return KeySpec{Type: t}
}

func RuneKey(r rune) KeySpec {
	return KeySpec{Type: tui.KeyRune, Rune: r}
}

func (b KeyBinding) WithHelp(key, desc string) KeyBinding {
	b.Help = KeyHelp{Key: key, Desc: desc}
	return b
}

func (b KeyBinding) Matches(msg tui.KeyMsg) bool {
	for _, spec := range b.Keys {
		if spec.Type == tui.KeyRune {
			if msg.Type == tui.KeyRune && msg.Rune == spec.Rune {
				return true
			}
		} else if msg.Type == spec.Type {
			return true
		}
	}
	return false
}
