package fileclient

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

// filenameCodec transforms path names only; file contents remain byte streams.
type filenameCodec struct {
	name     string
	encoding encoding.Encoding
}

func newFilenameCodec(name string) (filenameCodec, error) {
	codec := filenameCodec{name: name}
	switch name {
	case "", "UTF-8":
		codec.name = "UTF-8"
	case "GBK":
		codec.encoding = simplifiedchinese.GBK
	case "GB18030":
		codec.encoding = simplifiedchinese.GB18030
	case "Big5":
		codec.encoding = traditionalchinese.Big5
	case "Shift_JIS":
		codec.encoding = japanese.ShiftJIS
	case "EUC-KR":
		codec.encoding = korean.EUCKR
	case "ISO-8859-1":
		codec.encoding = charmap.ISO8859_1
	default:
		return filenameCodec{}, fmt.Errorf("unsupported filename encoding %q", name)
	}
	return codec, nil
}

func (c filenameCodec) encode(name string) (string, error) {
	if !utf8.ValidString(name) {
		return "", fmt.Errorf("filename is not valid UTF-8")
	}
	if c.encoding == nil {
		return name, nil
	}
	encoded, err := c.encoding.NewEncoder().String(name)
	if err != nil {
		return "", fmt.Errorf("encode filename as %s: %w", c.name, err)
	}
	return encoded, nil
}

func (c filenameCodec) decode(name string) (string, error) {
	if c.encoding == nil {
		if !utf8.ValidString(name) {
			return "", fmt.Errorf("filename is not valid UTF-8")
		}
		return name, nil
	}
	decoded, err := c.encoding.NewDecoder().String(name)
	if err != nil {
		return "", fmt.Errorf("decode %s filename: %w", c.name, err)
	}
	// x/text decoders silently replace malformed input. Validate those spans
	// without rejecting a genuine U+FFFD representable in GB18030.
	if strings.ContainsRune(decoded, utf8.RuneError) && !c.validReplacementSpans(name) {
		return "", fmt.Errorf("filename contains invalid %s bytes", c.name)
	}
	return decoded, nil
}

func (c filenameCodec) validReplacementSpans(name string) bool {
	for name != "" {
		consumed := 0
		for width := 1; width <= min(4, len(name)); width++ {
			source := name[:width]
			decoded, err := c.encoding.NewDecoder().String(source)
			if err != nil {
				continue
			}
			if strings.ContainsRune(decoded, utf8.RuneError) {
				roundTrip, err := c.encoding.NewEncoder().String(decoded)
				if err != nil || roundTrip != source {
					continue
				}
			}
			consumed = width
			break
		}
		if consumed == 0 {
			return false
		}
		name = name[consumed:]
	}
	return true
}
