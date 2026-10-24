package tag

// SeedTags returns the tags a fresh organization starts with, in display
// order. The welcome document goes under the first of them.
func SeedTags() []CreateInput {
	return []CreateInput{
		{TagName: "Production", Color: "green"},
		{TagName: "Staging", Color: "orange"},
		{TagName: "Incidents", Color: "blue"},
	}
}
