package graph

import "metro-wars/data"

type Node struct {
	ID           int                `json:"id"`
	Name         string             `json:"name"`
	X            float64            `json:"x"`
	Y            float64            `json:"y"`
	LineID       string             `json:"line_id"` // Изменили с int на string
	Type         data.NodeType      `json:"type"`
	Level        int                `json:"level"`
	Owner        int                `json:"owner"`
	HubID        int                `json:"hub_id"`
	IsDistrict   bool               `json:"is_district"`
	Population   int                `json:"population"`
	Garrison     int                `json:"garrison"`
	Army         int                `json:"army"`
	Resources    map[string]float64 `json:"resources"`
	UnderUpgrade bool               `json:"under_upgrade"`
	UpgradeTurns int                `json:"upgrade_turns"`
}

func NewNode(id int, name string, x, y float64, lineID string, nodeType data.NodeType) *Node {
	return &Node{
		ID:         id,
		Name:       name,
		X:          x,
		Y:          y,
		LineID:     lineID, // Теперь string
		Type:       nodeType,
		Level:      1,
		Owner:      0,
		HubID:      0,
		IsDistrict: false,
		Population: 100,
		Garrison:   10,
		Army:       0,
		Resources: map[string]float64{
			data.ResourceProvision: 0,
			data.ResourceMetal:     0,
			data.ResourceParts:     0,
		},
		UnderUpgrade: false,
		UpgradeTurns: 0,
	}
}
