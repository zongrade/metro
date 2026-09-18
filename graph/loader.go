package graph

import (
	"embed"
	"encoding/json"
	"metro/data"
	"os"
)

type MapData struct {
	Nodes      []NodeJSON        `json:"nodes"`
	Edges      []EdgeJSON        `json:"edges"`
	Hubs       []HubJSON         `json:"hubs"`
	LineColors map[string]string `json:"line_colors"`
}

type NodeJSON struct {
	ID          int          `json:"id"`
	Name        string       `json:"name"`
	X           float64      `json:"x"`
	Y           float64      `json:"y"`
	LineID      string       `json:"line_id"`
	Type        int          `json:"type"`
	Owner       int          `json:"owner"`
	LabelOffset *LabelOffset `json:"label_offset,omitempty"`
}

type LabelOffset struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type EdgeJSON struct {
	ID     int    `json:"id"`
	From   int    `json:"from"`
	To     int    `json:"to"`
	Length int    `json:"length"`
	Type   string `json:"type"`
	Time   int    `json:"time,omitempty"`
}

type HubJSON struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	StationIDs []int  `json:"station_ids"`
}

// LoadMapFromFile загружает карту из обычного файла на диске
func LoadMapFromFile(path string) (*Graph, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseMap(data)
}

func LoadMap(fs embed.FS, filename string) (*Graph, error) {
	fileData, err := fs.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return ParseMap(fileData)
}

func ParseMap(jsonBytes []byte) (*Graph, error) {
	var mapData MapData
	if err := json.Unmarshal(jsonBytes, &mapData); err != nil {
		return nil, err
	}

	g := NewGraph()
	g.LineColors = mapData.LineColors // <-- Копируем цвета линий

	// Создаём узлы
	for _, nodeJSON := range mapData.Nodes {
		node := NewNode(
			nodeJSON.ID,
			nodeJSON.Name,
			nodeJSON.X,
			nodeJSON.Y,
			nodeJSON.LineID, // Теперь string
			data.NodeType(nodeJSON.Type),
		)
		node.Owner = nodeJSON.Owner
		if nodeJSON.LabelOffset != nil {
			g.NodeLabelOffsets[node.ID] = struct{ X, Y float64 }{
				X: nodeJSON.LabelOffset.X,
				Y: nodeJSON.LabelOffset.Y,
			}
		}
		g.AddNode(node)
	}

	// Создаём рёбра
	for _, edgeJSON := range mapData.Edges {
		edge := NewEdge(
			edgeJSON.ID,
			edgeJSON.From,
			edgeJSON.To,
			edgeJSON.Length,
			edgeJSON.Time,
			edgeJSON.Type,
		)
		g.AddEdge(edge)
	}

	// Создаём хабы
	for _, hubJSON := range mapData.Hubs {
		hub := NewHub(
			hubJSON.ID,
			hubJSON.Name,
			hubJSON.StationIDs,
		)
		g.AddHub(hub)
	}

	return g, nil
}
