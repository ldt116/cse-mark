package student

// Model is a roster entry. MSSV is the primary key (_id). Each email maps to
// exactly one MSSV (enforced by a unique index on email).
type Model struct {
	MSSV  string `json:"mssv"  bson:"_id"`
	Name  string `json:"name"  bson:"name"`
	Email string `json:"email" bson:"email"`
}
