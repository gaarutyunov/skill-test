// Package domain holds the business entities of the report service. It has no
// dependencies on transport, persistence, or third-party libraries.
package domain

// Student is the business model of a student, independent of how it is fetched.
type Student struct {
	ID                 int64
	Name               string
	Email              string
	SystemAccess       bool
	Phone              string
	Gender             string
	DateOfBirth        string
	Class              string
	Section            string
	Roll               int64
	FatherName         string
	FatherPhone        string
	MotherName         string
	MotherPhone        string
	GuardianName       string
	GuardianPhone      string
	RelationOfGuardian string
	CurrentAddress     string
	PermanentAddress   string
	AdmissionDate      string
	ReporterName       string
}
