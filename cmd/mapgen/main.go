package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
)

type MetroAPI struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Lines []Line `json:"lines"`
}

type Line struct {
	ID       string    `json:"id"`
	HexColor string    `json:"hex_color"`
	Name     string    `json:"name"`
	Stations []Station `json:"stations"`
}

type Station struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Lat   float64 `json:"lat"`
	Lng   float64 `json:"lng"`
	Order int     `json:"order"`
	Line  LineRef `json:"line"` // <-- Цвет здесь!
}

type LineRef struct {
	ID       string `json:"id"`
	HexColor string `json:"hex_color"`
	Name     string `json:"name"`
}

type MapData struct {
	Nodes      []NodeJSON        `json:"nodes"`
	Edges      []EdgeJSON        `json:"edges"`
	Hubs       []HubJSON         `json:"hubs"`
	LineColors map[string]string `json:"line_colors"`
}

type NodeJSON struct {
	ID     int     `json:"id"`
	Name   string  `json:"name"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	LineID string  `json:"line_id"`
	Type   int     `json:"type"`
	Owner  int     `json:"owner"`
}

type EdgeJSON struct {
	ID     int    `json:"id"`
	From   int    `json:"from"`
	To     int    `json:"to"`
	Length int    `json:"length"`
	Type   string `json:"type"`
}

type HubJSON struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	StationIDs []int  `json:"station_ids"`
}

type MapDescription struct {
	name string
	url  string
}

var availableMaps = []MapDescription{
	{name: "Minsk", url: "https://api.hh.ru/metro/1002"},
	{name: "Moscow", url: "https://api.hh.ru/metro/1"},
}

func main() {
	// Флаг для выбора города. По умолчанию Minsk.
	cityFlag := flag.String("city", "Minsk", "City name to generate (e.g., Minsk, Moscow)")
	flag.Parse()

	// Ищем запрошенный город
	var target MapDescription
	found := false
	for _, m := range availableMaps {
		if m.name == *cityFlag {
			target = m
			found = true
			break
		}
	}

	if !found {
		fmt.Printf("City '%s' not found.\nAvailable cities: ", *cityFlag)
		for _, m := range availableMaps {
			fmt.Printf("%s ", m.name)
		}
		fmt.Println()
		os.Exit(1)
	}

	fmt.Printf("Fetching data for %s from %s...\n", target.name, target.url)
	resp, err := http.Get(target.url)
	if err != nil {
		fmt.Printf("Error fetching API: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response: %v\n", err)
		os.Exit(1)
	}

	var metro MetroAPI
	if err := json.Unmarshal(body, &metro); err != nil {
		fmt.Printf("Error parsing JSON: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Metro: %s\n", metro.Name)
	fmt.Printf("Lines: %d\n", len(metro.Lines))

	for _, line := range metro.Lines {
		fmt.Printf("  Line %s: %s (color: %s, %d stations)\n",
			line.ID, line.Name, line.HexColor, len(line.Stations))
	}

	mapData := convertToMapData(metro)

	separateCloseNodes(&mapData)

	output, err := json.MarshalIndent(mapData, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling: %v\n", err)
		os.Exit(1)
	}

	filename := fmt.Sprintf("assets/maps/%s.json", target.name)
	err = os.WriteFile(filename, output, 0644)
	if err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nMap '%s' generated successfully and saved to %s!\n", target.name, filename)
	fmt.Printf("Total stations: %d\n", len(mapData.Nodes))
}

// separateCloseNodes раздвигает узлы, чьи круги пересекаются или касаются
func separateCloseNodes(mapData *MapData) {
	// Радиус узла + обводка + небольшой запас
	nodeRadius := 14.0
	minDistance := nodeRadius*2 + 4 // 32px минимум между центрами

	changed := true
	iterations := 0
	maxIterations := 10

	for changed && iterations < maxIterations {
		changed = false
		iterations++

		for i := 0; i < len(mapData.Nodes); i++ {
			for j := i + 1; j < len(mapData.Nodes); j++ {
				nodeA := &mapData.Nodes[i]
				nodeB := &mapData.Nodes[j]

				dx := nodeB.X - nodeA.X
				dy := nodeB.Y - nodeA.Y
				distance := math.Sqrt(dx*dx + dy*dy)

				if distance < minDistance && distance > 0 {
					// Раздвигаем узлы в противоположные стороны
					offset := (minDistance - distance) / 2
					ratio := offset / distance

					nodeA.X -= dx * ratio
					nodeA.Y -= dy * ratio
					nodeB.X += dx * ratio
					nodeB.Y += dy * ratio

					changed = true
				}
			}
		}
	}

	fmt.Printf("Node separation: %d iterations\n", iterations)
}

func convertToMapData(metro MetroAPI) MapData {
	// Собираем все станции
	type FlatStation struct {
		Name   string
		Lat    float64
		Lng    float64
		LineID string
		Color  string
		Order  int
	}

	var allStations []FlatStation
	lineColors := make(map[string]string)

	for _, line := range metro.Lines {
		lineColors[line.ID] = line.HexColor

		for _, station := range line.Stations {
			allStations = append(allStations, FlatStation{
				Name:   station.Name,
				Lat:    station.Lat,
				Lng:    station.Lng,
				LineID: line.ID,
				Color:  line.HexColor,
				Order:  station.Order,
			})
		}
	}

	// Находим bounding box
	minLat, maxLat := allStations[0].Lat, allStations[0].Lat
	minLng, maxLng := allStations[0].Lng, allStations[0].Lng

	for _, s := range allStations {
		if s.Lat < minLat {
			minLat = s.Lat
		}
		if s.Lat > maxLat {
			maxLat = s.Lat
		}
		if s.Lng < minLng {
			minLng = s.Lng
		}
		if s.Lng > maxLng {
			maxLng = s.Lng
		}
	}

	fmt.Printf("\nBounding box:\n")
	fmt.Printf("  Lat: %.6f to %.6f\n", minLat, maxLat)
	fmt.Printf("  Lng: %.6f to %.6f\n", minLng, maxLng)

	// Находим максимальное расстояние для масштаба
	maxDist := math.Sqrt(math.Pow(maxLat-minLat, 2) + math.Pow(maxLng-minLng, 2))
	targetPixels := 1400.0 // Чтобы карта влезла с отступами
	scale := targetPixels / maxDist

	fmt.Printf("  Max distance: %.4f degrees\n", maxDist)
	fmt.Printf("  Scale: %.2f px/degree\n", scale)

	// Центр
	centerLat := (minLat + maxLat) / 2
	centerLng := (minLng + maxLng) / 2

	// Смещение в центр экрана
	offsetX := 960.0
	offsetY := 540.0

	fmt.Printf("  Center: lat %.6f, lng %.6f\n", centerLat, centerLng)

	mapData := MapData{
		Nodes:      []NodeJSON{},
		Edges:      []EdgeJSON{},
		Hubs:       []HubJSON{},
		LineColors: lineColors,
	}

	stationsByName := make(map[string][]NodeJSON)

	nodeID := 1
	for _, line := range metro.Lines {
		sort.Slice(line.Stations, func(i, j int) bool {
			return line.Stations[i].Order < line.Stations[j].Order
		})

		prevNodeID := -1
		for _, station := range line.Stations {
			// Преобразование координат:
			// X растёт вправо (с запада на восток)
			// Y растёт вниз (с севера на юг)
			x := offsetX + (station.Lng-centerLng)*scale
			y := offsetY + (centerLat-station.Lat)*scale // Инвертируем Y!

			node := NodeJSON{
				ID:     nodeID,
				Name:   station.Name,
				X:      math.Round(x*10) / 10,
				Y:      math.Round(y*10) / 10,
				LineID: line.ID,
				Type:   0,
				Owner:  0,
			}

			mapData.Nodes = append(mapData.Nodes, node)
			stationsByName[station.Name] = append(stationsByName[station.Name], node)

			if prevNodeID != -1 {
				edge := EdgeJSON{
					ID:     len(mapData.Edges) + 1,
					From:   prevNodeID,
					To:     nodeID,
					Length: 1,
					Type:   "normal",
				}
				mapData.Edges = append(mapData.Edges, edge)
			}
			prevNodeID = nodeID
			nodeID++
		}
	}

	// Пересадки
	hubID := 1
	for name, nodes := range stationsByName {
		if len(nodes) > 1 {
			lineSet := make(map[string]bool)
			for _, n := range nodes {
				lineSet[n.LineID] = true
			}
			if len(lineSet) > 1 {
				nodeIDs := make([]int, len(nodes))
				for i, n := range nodes {
					nodeIDs[i] = n.ID
				}

				hub := HubJSON{
					ID:         hubID,
					Name:       name,
					StationIDs: nodeIDs,
				}
				mapData.Hubs = append(mapData.Hubs, hub)

				for i := 0; i < len(nodeIDs); i++ {
					for j := i + 1; j < len(nodeIDs); j++ {
						edge := EdgeJSON{
							ID:     len(mapData.Edges) + 1,
							From:   nodeIDs[i],
							To:     nodeIDs[j],
							Length: 1,
							Type:   "transfer",
						}
						mapData.Edges = append(mapData.Edges, edge)
					}
				}
				hubID++
			}
		}
	}

	return mapData
}
