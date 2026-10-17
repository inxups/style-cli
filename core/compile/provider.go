package compile

// HasProvider identifies entries that require the model-service semantics.
func (entry Entry) HasProvider() bool {
	if entry.When.Provider != nil {
		return true
	}
	for _, field := range entry.Needs {
		if field == "provider" {
			return true
		}
	}
	bodies := []Body{entry.Body}
	if entry.ModelBody != nil {
		bodies = append(bodies, *entry.ModelBody)
	}
	for _, body := range bodies {
		for _, part := range body.Parts {
			if part.Variable != nil && part.Variable.Name == "provider" {
				return true
			}
		}
	}
	return false
}
