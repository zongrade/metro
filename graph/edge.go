package graph

type Edge struct {
	ID     int    `json:"id"`
	From   int    `json:"from"`   // ID узла A
	To     int    `json:"to"`     // ID узла B
	Length int    `json:"length"` // Время прохождения в ходах
	Type   string `json:"type"`   // "normal" или "transfer" (пересадка)
}

func NewEdge(id, from, to, length int, edgeType string) *Edge {
	return &Edge{
		ID:     id,
		From:   from,
		To:     to,
		Length: length,
		Type:   edgeType,
	}
}
