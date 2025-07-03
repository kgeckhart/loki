package pushrequest

type ExtractType string

// TODO find an easier way to share this so we don't have guest/host duplication
const (
	ExtractTypeLabel    ExtractType = "label"
	ExtractTypeMetadata ExtractType = "metadata"
)
