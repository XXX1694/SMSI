package linkedin

import (
	"strings"
	"unicode"
)

// reserved characters of LinkedIn's "little text" commentary format.
const reserved = `\|{}@[]()<>#*_~`

// FormatCommentary escapes reserved characters and turns #hashtags into
// LinkedIn hashtag templates ({hashtag|\#|tag}) so they render as links.
func FormatCommentary(text string) string {
	var sb strings.Builder
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '#' && (i == 0 || unicode.IsSpace(runes[i-1])) {
			j := i + 1
			for j < len(runes) && (unicode.IsLetter(runes[j]) || unicode.IsDigit(runes[j])) {
				j++
			}
			if j > i+1 {
				sb.WriteString(`{hashtag|\#|`)
				sb.WriteString(string(runes[i+1 : j]))
				sb.WriteString("}")
				i = j - 1
				continue
			}
		}
		if strings.ContainsRune(reserved, r) {
			sb.WriteByte('\\')
		}
		sb.WriteRune(r)
	}
	return sb.String()
}
