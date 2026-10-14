package compile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Encode produces deterministic UTF-8 JSON with a final newline. Map keys are
// sorted by encoding/json; entry arrays are already sorted by Build.
func Encode(index Index) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(index); err != nil {
		return nil, fmt.Errorf("compile: encode index: %w", err)
	}
	return output.Bytes(), nil
}

func (order Order) MarshalJSON() ([]byte, error) {
	return json.Marshal([3]any{order.Priority, order.PackageSeq, order.RuleIndex})
}

func (order *Order) UnmarshalJSON(data []byte) error {
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil || len(items) != 3 {
		return fmt.Errorf("compile: order must contain three numbers")
	}
	var decoded Order
	if err := json.Unmarshal(items[0], &decoded.Priority); err != nil {
		return err
	}
	if err := json.Unmarshal(items[1], &decoded.PackageSeq); err != nil {
		return err
	}
	if err := json.Unmarshal(items[2], &decoded.RuleIndex); err != nil {
		return err
	}
	*order = decoded
	return nil
}

func (index *Index) UnmarshalJSON(data []byte) error {
	type indexWire Index
	var decoded indexWire
	if err := decodeStrict(data, &decoded); err != nil {
		return err
	}
	if decoded.Format != Format {
		return fmt.Errorf("compile: unsupported index format %d", decoded.Format)
	}
	for _, events := range decoded.Harnesses {
		for event, entries := range events {
			for i := range entries {
				entries[i].When.Event = event
			}
			events[event] = entries
		}
	}
	*index = Index(decoded)
	return nil
}

type bodyWire struct {
	Static *string            `json:"static,omitempty"`
	Parts  *[]json.RawMessage `json:"parts,omitempty"`
}

// MarshalJSON uses strings for literals and {var, default} for variables.
func (body Body) MarshalJSON() ([]byte, error) {
	wire := bodyWire{Static: body.Static}
	if body.Static != nil {
		if body.Parts != nil {
			return nil, fmt.Errorf("compile: static body cannot have parts")
		}
		return marshalNoEscape(wire)
	}
	parts := make([]json.RawMessage, 0, len(body.Parts))
	for _, part := range body.Parts {
		var data []byte
		var err error
		if part.Variable == nil {
			data, err = marshalNoEscape(part.Text)
		} else {
			data, err = marshalNoEscape(struct {
				Name    string  `json:"var"`
				Default *string `json:"default,omitempty"`
			}{Name: part.Variable.Name, Default: part.Variable.Default})
		}
		if err != nil {
			return nil, err
		}
		parts = append(parts, data)
	}
	wire.Parts = &parts
	return marshalNoEscape(wire)
}

func marshalNoEscape(value any) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(output.Bytes(), []byte("\n")), nil
}

// UnmarshalJSON decodes the compiled fragments, rejecting ambiguous bodies.
func (body *Body) UnmarshalJSON(data []byte) error {
	var wire bodyWire
	if err := decodeStrict(data, &wire); err != nil {
		return err
	}
	if (wire.Static == nil) == (wire.Parts == nil) {
		return fmt.Errorf("compile: body requires exactly one of static or parts")
	}
	decoded := Body{Static: wire.Static}
	if wire.Parts != nil {
		decoded.Parts = make([]Fragment, 0, len(*wire.Parts))
		for _, raw := range *wire.Parts {
			var literal string
			if len(raw) > 0 && raw[0] == '"' {
				if err := json.Unmarshal(raw, &literal); err != nil {
					return err
				}
				decoded.Parts = append(decoded.Parts, Fragment{Text: literal})
				continue
			}
			var variable struct {
				Name    string  `json:"var"`
				Default *string `json:"default,omitempty"`
			}
			if err := decodeStrict(raw, &variable); err != nil || variable.Name == "" {
				return fmt.Errorf("compile: invalid runtime variable fragment")
			}
			decoded.Parts = append(decoded.Parts, Fragment{Variable: &Variable{Name: variable.Name, Default: variable.Default}})
		}
	}
	*body = decoded
	return nil
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("compile: unexpected trailing JSON")
	}
	return nil
}
