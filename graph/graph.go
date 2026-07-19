package graph

type Graph struct {
	Nodes      map[int]*Node
	Edges      map[int]*Edge
	Hubs       map[int]*Hub
	LineColors map[string]string // line_id -> hex_color
	// Индексы для быстрого поиска
	NodeEdges map[int][]int // ID узла -> список ID рёбер
	HubNodes  map[int][]int // ID узла -> ID хаба (если есть)
}

func NewGraph() *Graph {
	return &Graph{
		Nodes:      make(map[int]*Node),
		Edges:      make(map[int]*Edge),
		Hubs:       make(map[int]*Hub),
		LineColors: make(map[string]string),
		NodeEdges:  make(map[int][]int),
		HubNodes:   make(map[int][]int),
	}
}

// AddNode добавляет узел в граф
func (g *Graph) AddNode(node *Node) {
	g.Nodes[node.ID] = node
}

// AddEdge добавляет ребро в граф
func (g *Graph) AddEdge(edge *Edge) {
	g.Edges[edge.ID] = edge
	g.NodeEdges[edge.From] = append(g.NodeEdges[edge.From], edge.ID)
	g.NodeEdges[edge.To] = append(g.NodeEdges[edge.To], edge.ID)
}

// AddHub добавляет пересадочный узел
func (g *Graph) AddHub(hub *Hub) {
	g.Hubs[hub.ID] = hub
	for _, stationID := range hub.StationIDs {
		g.HubNodes[stationID] = append(g.HubNodes[stationID], hub.ID)
	}
}

// GetNeighbors возвращает соседние узлы
func (g *Graph) GetNeighbors(nodeID int) []int {
	var neighbors []int
	for _, edgeID := range g.NodeEdges[nodeID] {
		edge := g.Edges[edgeID]
		if edge.From == nodeID {
			neighbors = append(neighbors, edge.To)
		} else {
			neighbors = append(neighbors, edge.From)
		}
	}
	return neighbors
}

// GetEdgeBetween возвращает ребро между двумя узлами (если есть)
func (g *Graph) GetEdgeBetween(nodeA, nodeB int) *Edge {
	for _, edgeID := range g.NodeEdges[nodeA] {
		edge := g.Edges[edgeID]
		if (edge.From == nodeA && edge.To == nodeB) || (edge.From == nodeB && edge.To == nodeA) {
			return edge
		}
	}
	return nil
}

// GetHubForNode возвращает хаб, к которому принадлежит узел
func (g *Graph) GetHubForNode(nodeID int) *Hub {
	hubIDs, exists := g.HubNodes[nodeID]
	if !exists || len(hubIDs) == 0 {
		return nil
	}
	return g.Hubs[hubIDs[0]]
}

// FindPath находит кратчайший путь между двумя узлами (BFS)
func (g *Graph) FindPath(from, to int) []int {
	if from == to {
		return []int{from}
	}

	visited := make(map[int]bool)
	queue := []int{from}
	parent := make(map[int]int)
	visited[from] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, neighbor := range g.GetNeighbors(current) {
			if !visited[neighbor] {
				visited[neighbor] = true
				parent[neighbor] = current
				queue = append(queue, neighbor)

				if neighbor == to {
					// Восстанавливаем путь
					var path []int
					for node := to; node != from; node = parent[node] {
						path = append([]int{node}, path...)
					}
					path = append([]int{from}, path...)
					return path
				}
			}
		}
	}

	return nil // Путь не найден
}
