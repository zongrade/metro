package graph

import "metro/data"

type Hub struct {
	ID         int          `json:"id"`
	Name       string       `json:"name"`
	StationIDs []int        `json:"station_ids"`
	Controller int          `json:"controller"` // ID фракции (0 = никтo)
	Type       data.HubType `json:"type"`
	IsLocked   bool         `json:"is_locked"` // Нельзя сменить тип (идёт война)
}

func NewHub(id int, name string, stationIDs []int) *Hub {
	return &Hub{
		ID:         id,
		Name:       name,
		StationIDs: stationIDs,
		Controller: 0,
		Type:       data.HubMixed,
		IsLocked:   false,
	}
}

// HasFullControl проверяет, контролирует ли фракция все станции узла
func (h *Hub) HasFullControl(nodes map[int]*Node, factionID int) bool {
	for _, stationID := range h.StationIDs {
		node, exists := nodes[stationID]
		if !exists || node.Owner != factionID {
			return false
		}
	}
	return true
}
