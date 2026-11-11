package deck

import (
	"bytes"
	"encoding/json"
)

// Paint is a fill: a colour (a JSON string, as in v1 decks) or a two-stop
// linear gradient (a JSON object).
type Paint struct {
	Color    string
	Gradient *Gradient
}

// Gradient is a linear gradient across an element's box. Angle is in
// degrees, clockwise from left-to-right (90 runs top to bottom); each stop is
// #RRGGBB or a theme colour name, so a theme change recolours it.
type Gradient struct {
	Type  string   `json:"type"`
	Angle float64  `json:"angle"`
	Stops []string `json:"stops"`
}

func (p Paint) MarshalJSON() ([]byte, error) {
	if p.Gradient != nil {
		return json.Marshal(p.Gradient)
	}
	return json.Marshal(p.Color)
}

func (p *Paint) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '{' {
		var g Gradient
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&g); err != nil {
			return err
		}
		*p = Paint{Gradient: &g}
		return nil
	}
	var c string
	if err := json.Unmarshal(b, &c); err != nil {
		return err
	}
	*p = Paint{Color: c}
	return nil
}
