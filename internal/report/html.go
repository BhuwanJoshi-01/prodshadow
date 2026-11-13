package report

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"math"
	"strings"
)

//go:embed report.html.tmpl
var htmlTemplate string

var htmlFuncs = template.FuncMap{
	"ms":    fmtMS,
	"ratio": fmtRatio,
	"pct":   fmtPct,
	"cv":    fmtCV,
	"int":   func(n int) string { return fmtInt(int64(n)) },
	"int64": fmtInt,
	"loc":   location,
	"upper": strings.ToUpper,
	"join":  strings.Join,
	"inc":   func(i int) int { return i + 1 },
}

var parsedHTML = template.Must(template.New("report").Funcs(htmlFuncs).Parse(htmlTemplate))

// HTML renders a self-contained page with no external resources, suitable
// for archiving as a CI artifact.
func (r *Report) HTML() (string, error) {
	var buf bytes.Buffer
	data := struct {
		*Report
		Disclaimer string
		Chart      template.HTML
	}{r, Disclaimer, r.timelineSVG()}
	if err := parsedHTML.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// timelineSVG draws the worst candidate phase: throughput as bars and the
// longest operation per second as a line on a logarithmic scale.
func (r *Report) timelineSVG() template.HTML {
	var worst *Phase
	for i := range r.Phases {
		p := &r.Phases[i]
		if p.Kind == "candidate" && (worst == nil || maxBucket(p) > maxBucket(worst)) {
			worst = p
		}
	}
	if worst == nil || len(worst.Timeline) == 0 {
		return ""
	}
	const w, h, pad = 720.0, 180.0, 28.0
	n := float64(len(worst.Timeline))
	bw := (w - 2*pad) / n
	maxOps, maxLat := 1, 1.0
	for _, b := range worst.Timeline {
		maxOps = max(maxOps, b.Ops)
		maxLat = math.Max(maxLat, b.MaxObservedMS)
	}
	logMax := math.Log10(maxLat*1.2 + 1)
	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg viewBox="0 0 %.0f %.0f" role="img" aria-label="Candidate timeline: operations per second and longest operation">`, w, h)
	if m := worst.Migration; m != nil {
		x0 := pad + (m.OffsetMS/1000)*bw
		x1 := pad + ((m.OffsetMS+m.DurationMS)/1000)*bw
		fmt.Fprintf(&sb, `<rect class="mig" x="%.1f" y="%.0f" width="%.1f" height="%.0f"><title>migration running</title></rect>`, x0, pad/2, math.Max(2, x1-x0), h-pad*1.5)
	}
	var pts []string
	for i, b := range worst.Timeline {
		x := pad + float64(i)*bw
		bh := float64(b.Ops) / float64(maxOps) * (h - 2*pad)
		fmt.Fprintf(&sb, `<rect class="ops" x="%.1f" y="%.1f" width="%.1f" height="%.1f"><title>%.0fs: %d ops, max %s</title></rect>`,
			x+1, h-pad-bh, math.Max(1, bw-2), bh, b.OffsetSec, b.Ops, fmtMS(b.MaxObservedMS))
		y := h - pad - math.Log10(b.MaxObservedMS+1)/logMax*(h-2*pad)
		pts = append(pts, fmt.Sprintf("%.1f,%.1f", x+bw/2, y))
	}
	fmt.Fprintf(&sb, `<polyline class="lat" points="%s"/>`, strings.Join(pts, " "))
	fmt.Fprintf(&sb, `<text x="%.0f" y="%.0f">0 s</text><text x="%.0f" y="%.0f" text-anchor="end">%.0f s</text>`, pad, h-8, w-pad, h-8, n)
	fmt.Fprintf(&sb, `<text x="%.0f" y="14">max %s</text></svg>`, pad, fmtMS(maxLat))
	return template.HTML(sb.String())
}
