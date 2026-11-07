package locales

import (
	"fmt"
	"strings"
)

// Text retains a message's arguments until it is displayed. The zero value
// represents an absent notice; raw remote output and saved history stay literal.
type Text struct{ render func() string }

func (t Text) String() string {
	if t.render == nil {
		return ""
	}
	return t.render()
}

func (t Text) Empty() bool { return t.String() == "" }

func (t Text) TrimSpace() Text {
	return Deferred(func() string { return strings.TrimSpace(t.String()) })
}

func Deferred(render func() string) Text { return Text{render: render} }

func Raw(text string) Text {
	return Deferred(func() string { return text })
}

func ErrorText(err error) Text { return Deferred(err.Error) }

func Format(format func() string, args ...any) Text {
	return Deferred(func() string { return fmt.Sprintf(format(), args...) })
}

func Concat(parts ...Text) Text {
	return Deferred(func() string {
		var text strings.Builder
		for _, part := range parts {
			text.WriteString(part.String())
		}
		return text.String()
	})
}
