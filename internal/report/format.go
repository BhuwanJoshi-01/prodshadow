package report

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// esc makes text safe inside a Markdown table cell.
func esc(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", `\|`)
	return s
}

func fmtMS(v float64) string {
	switch {
	case math.IsNaN(v):
		return "–"
	case v >= 10000:
		return fmt.Sprintf("%.1f s", v/1000)
	case v >= 100:
		return fmt.Sprintf("%.0f ms", v)
	case v >= 1:
		return fmt.Sprintf("%.1f ms", v)
	default:
		return fmt.Sprintf("%.2f ms", v)
	}
}

func fmtRatio(v *float64) string {
	if v == nil {
		return "–"
	}
	return fmt.Sprintf("%.2f×", *v)
}

func fmtPct(v *float64) string {
	if v == nil {
		return "–"
	}
	return fmt.Sprintf("%+.1f%%", *v)
}

func fmtCV(v *float64) string {
	if v == nil {
		return "–"
	}
	return fmt.Sprintf("%.0f%%", *v*100)
}

func fmtInt(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func fmtBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func fmtDuration(sec float64) string {
	return (time.Duration(sec * float64(time.Second))).Round(time.Second).String()
}

func location(file string, line int) string {
	if file == "" {
		return "–"
	}
	if line > 0 {
		return fmt.Sprintf("%s:%d", file, line)
	}
	return file
}
