package course

type Status string

const (
	StatusActive   Status = "active"
	StatusStale    Status = "stale"
	StatusInactive Status = "inactive"
)

type Model struct {
	Id         string `json:"course" bson:"course"`
	Link       string `json:"link" bson:"link"`
	ByTeleId   int64  `json:"by_id" bson:"by_id"`
	ByTeleUser string `json:"by_user,omitempty" bson:"by_user,omitempty"`
	UpdatedAt  int64  `json:"updated_at" bson:"updated_at"`
	RecordCnt  int64  `json:"record_cnt" bson:"record_cnt"`
	Status     Status `json:"status,omitempty" bson:"status,omitempty"`
}

// EffectiveStatus treats the zero value (legacy docs without the field) as
// active so existing courses keep polling after the deploy.
func (m Model) EffectiveStatus() Status {
	if m.Status == "" {
		return StatusActive
	}
	return m.Status
}
