package domain

// Report is a generated student report artifact.
type Report struct {
	StudentID   int64
	StudentName string
	FileName    string
	ContentType string
	Content     []byte
	PageCount   int
}

// Size returns the report payload size in bytes.
func (r Report) Size() int { return len(r.Content) }
