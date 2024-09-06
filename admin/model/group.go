package model

type Group struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`

	Input   string `json:"input"`
	Output  string `json:"output"`
	Outputs string `json:"outputs"`

	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`

	Cfg string `json:"cfg"`
}

func (g *Group) TableName() string {
	return "group"
}
