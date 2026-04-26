package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/mattn/go-runewidth"
)

// printTable writes a column-aligned table using display width, so cells
// containing east-asian / full-width characters align correctly.
//
// indent is prepended to every line. Header underline (---) is generated from
// header text width. Columns are separated by two spaces.
func printTable(w io.Writer, indent string, header []string, rows [][]string) {
	if len(header) == 0 {
		return
	}
	cols := len(header)
	widths := make([]int, cols)
	for i, h := range header {
		widths[i] = runewidth.StringWidth(h)
	}
	for _, row := range rows {
		for i := 0; i < cols && i < len(row); i++ {
			if rw := runewidth.StringWidth(row[i]); rw > widths[i] {
				widths[i] = rw
			}
		}
	}

	write := func(cells []string) {
		var sb strings.Builder
		sb.WriteString(indent)
		for i, c := range cells {
			sb.WriteString(c)
			if i < cols-1 {
				if pad := widths[i] - runewidth.StringWidth(c); pad > 0 {
					sb.WriteString(strings.Repeat(" ", pad))
				}
				sb.WriteString("  ")
			}
		}
		sb.WriteByte('\n')
		fmt.Fprint(w, sb.String())
	}

	write(header)

	underline := make([]string, cols)
	for i, h := range header {
		underline[i] = strings.Repeat("-", runewidth.StringWidth(h))
	}
	write(underline)

	for _, row := range rows {
		padded := make([]string, cols)
		for i := 0; i < cols; i++ {
			if i < len(row) {
				padded[i] = row[i]
			}
		}
		write(padded)
	}
}
