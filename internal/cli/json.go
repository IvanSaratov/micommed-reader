package cli

import (
	"bufio"
	"encoding/json"
	"io"
)

func (r *report) writeJSON(dst io.Writer) error {
	// Записываем дни по одному, чтобы размер архива не определял размер временной JSON-модели.
	w := bufio.NewWriter(dst)
	enc := json.NewEncoder(w)
	w.WriteString(`{"schema_version":1`)
	for _, f := range []struct {
		name string
		v    any
	}{
		{"profile", r.profile}, {"decoder_version", r.version}, {"sources", r.sources()},
		{"period", periodValues(r.period)}, {"diagnostics", r.diagnostics},
	} {
		w.WriteString(`,"` + f.name + `":`)
		if err := enc.Encode(f.v); err != nil {
			return err
		}
	}
	w.WriteString(`,"days":[`)
	for i, d := range r.days {
		if i > 0 {
			w.WriteByte(',')
		}
		if err := enc.Encode(dayValues(d)); err != nil {
			return err
		}
	}
	w.WriteString("]}\n")
	return w.Flush()
}
