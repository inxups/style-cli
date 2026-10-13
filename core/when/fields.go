package when

// FieldReader reads an event-derived condition field. False means unavailable;
// available empty strings remain distinguishable from missing data.
type FieldReader interface {
	ReadConditionField(field string) (value string, available bool)
}

// FieldMap adapts an already-normalized set of field values to FieldReader.
// Adapters can implement FieldReader directly to read provider payloads using
// agenthooks.Event.RawField and their typed event data.
type FieldMap map[string]string

func (fields FieldMap) ReadConditionField(field string) (string, bool) {
	value, ok := fields[field]
	return value, ok
}

// FieldFunc adapts a function to FieldReader.
type FieldFunc func(field string) (value string, available bool)

func (read FieldFunc) ReadConditionField(field string) (string, bool) {
	if read == nil {
		return "", false
	}
	return read(field)
}
