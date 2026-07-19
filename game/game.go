package game

import (
	"bytes"
	"embed"
	"image/color"
	"log"
	"math"
	"sort"

	"metro-wars/camera"
	"metro-wars/data"
	"metro-wars/graph"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type Game struct {
	camera       *camera.Camera
	graph        *graph.Graph
	fontFace     text.Face
	fontCache    map[float64]text.Face
	baseFontSize float64
	fontsFS      embed.FS
}

func New(mapsFS embed.FS, fontsFS embed.FS) (*Game, error) {
	g := &Game{
		camera:       camera.New(1920, 1080),
		fontsFS:      fontsFS,
		fontCache:    make(map[float64]text.Face),
		baseFontSize: 14.0,
	}

	// Загружаем карту
	var err error
	g.graph, err = graph.LoadMap(mapsFS, "assets/maps/test_map.json")
	if err != nil {
		log.Printf("Warning: Could not load map: %v. Using empty graph.", err)
		g.graph = graph.NewGraph()
	}

	// Загружаем шрифт
	g.fontFace, err = g.loadFont("assets/fonts/Roboto-Regular.ttf", 14)
	if err != nil {
		log.Printf("Warning: Could not load font: %v. Using fallback.", err)
	}

	return g, nil
}

// Получаем шрифт нужного размера
func (g *Game) getFontFace(fontSize float64) text.Face {
	if face, exists := g.fontCache[fontSize]; exists {
		return face
	}

	face, err := g.loadFont("assets/fonts/Roboto-Regular.ttf", fontSize)
	if err != nil {
		return g.fontFace // Возвращаем базовый шрифт в случае ошибки
	}

	g.fontCache[fontSize] = face
	return face
}

func (g *Game) loadFont(path string, size float64) (text.Face, error) {
	fontData, err := g.fontsFS.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// text.NewGoTextFaceSource принимает io.Reader и возвращает (*GoTextFaceSource, error)
	source, err := text.NewGoTextFaceSource(bytes.NewReader(fontData))
	if err != nil {
		return nil, err
	}

	// Создаём лицо шрифта с нужным размером
	face := &text.GoTextFace{
		Source: source,
		Size:   size,
	}

	return face, nil
}

func (g *Game) Update() error {
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonMiddle) {
		g.camera.TogglePanning()
	}

	g.camera.Update()

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return ebiten.Termination
	}

	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{20, 20, 25, 255})

	if len(g.graph.Nodes) > 0 {
		g.drawGraph(screen)
	} else {
		g.drawWorldMarker(screen)
	}

	g.drawCameraModeIndicator(screen)
}

type edgeWithLine struct {
	edge   *graph.Edge
	lineID string
	nodeA  *graph.Node
	nodeB  *graph.Node
}

type screenPoint struct {
	x, y float64
}

func (g *Game) drawGraph(screen *ebiten.Image) {
	// 1. Сначала рисуем хабы (фон)
	for _, hub := range g.graph.Hubs {
		g.drawHub(screen, hub)
	}

	// 2. Собираем рёбра с информацией о линии
	var edges []edgeWithLine
	for _, edge := range g.graph.Edges {
		nodeA := g.graph.Nodes[edge.From]
		nodeB := g.graph.Nodes[edge.To]
		edges = append(edges, edgeWithLine{
			edge:   edge,
			lineID: nodeA.LineID,
			nodeA:  nodeA,
			nodeB:  nodeB,
		})
	}

	// 3. Сортируем по LineID — фиксированный порядок отрисовки
	sort.Slice(edges, func(i, j int) bool {
		return edges[i].lineID < edges[j].lineID
	})

	// 4. Рисуем рёбра с разрывами на пересечениях
	for i, e := range edges {
		var intersections []screenPoint
		for j := i + 1; j < len(edges); j++ {
			other := edges[j]
			if p, ok := lineIntersection(e.nodeA, e.nodeB, other.nodeA, other.nodeB); ok {
				sx, sy := g.camera.WorldToScreen(p.x, p.y)
				intersections = append(intersections, screenPoint{x: sx, y: sy})
			}
		}
		g.drawEdgeWithGaps(screen, e.nodeA, e.nodeB, e.edge, intersections)
	}

	// 5. Собираем узлы в срез и сортируем по ID для фиксированного порядка
	var nodes []*graph.Node
	for _, node := range g.graph.Nodes {
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})

	// 6. Рисуем узлы и их метки в фиксированном порядке
	for _, node := range nodes {
		g.drawNode(screen, node)
		g.drawNodeLabel(screen, node)
	}
}

// lineIntersection находит точку пересечения двух отрезков в МИРОВЫХ координатах.
// Возвращает (точка, true) если отрезки пересекаются внутри себя (не у концов).
func lineIntersection(a1, a2, b1, b2 *graph.Node) (struct{ x, y float64 }, bool) {
	x1, y1 := a1.X, a1.Y
	x2, y2 := a2.X, a2.Y
	x3, y3 := b1.X, b1.Y
	x4, y4 := b2.X, b2.Y

	denom := (x1-x2)*(y3-y4) - (y1-y2)*(x3-x4)
	if math.Abs(denom) < 0.0001 {
		return struct{ x, y float64 }{}, false // параллельны или совпадают
	}

	t := ((x1-x3)*(y3-y4) - (y1-y3)*(x3-x4)) / denom
	u := -((x1-x2)*(y1-y3) - (y1-y2)*(x1-x3)) / denom

	// Пересечение должно быть внутри обоих отрезков
	if t < 0 || t > 1 || u < 0 || u > 1 {
		return struct{ x, y float64 }{}, false
	}

	// Игнорируем пересечения у самых концов — там узлы всё равно перекроют
	const edgeMargin = 0.08
	if t < edgeMargin || t > 1-edgeMargin || u < edgeMargin || u > 1-edgeMargin {
		return struct{ x, y float64 }{}, false
	}

	return struct{ x, y float64 }{
		x: x1 + t*(x2-x1),
		y: y1 + t*(y2-y1),
	}, true
}

func (g *Game) drawEdgeWithGaps(screen *ebiten.Image, nodeA, nodeB *graph.Node, edge *graph.Edge, intersections []screenPoint) {
	screenXA, screenYA := g.camera.WorldToScreen(nodeA.X, nodeA.Y)
	screenXB, screenYB := g.camera.WorldToScreen(nodeB.X, nodeB.Y)

	// Цвет ребра
	hexColor := g.graph.LineColors[nodeA.LineID]
	c := hexToColor(hexColor)
	if edge.Type == "transfer" {
		c = color.RGBA{255, 255, 255, 180}
	}

	// Если нет пересечений — рисуем обычную линию
	if len(intersections) == 0 {
		vector.StrokeLine(screen,
			float32(screenXA), float32(screenYA),
			float32(screenXB), float32(screenYB),
			3, c, true)
		return
	}

	// Сортируем точки пересечения по расстоянию от начала отрезка
	sort.Slice(intersections, func(i, j int) bool {
		di := (intersections[i].x-screenXA)*(intersections[i].x-screenXA) +
			(intersections[i].y-screenYA)*(intersections[i].y-screenYA)
		dj := (intersections[j].x-screenXA)*(intersections[j].x-screenXA) +
			(intersections[j].y-screenYA)*(intersections[j].y-screenYA)
		return di < dj
	})

	// Рисуем отрезки с пропусками в точках пересечения
	gapRadius := 9.0 // радиус разрыва в экранных пикселях (фиксирован — виден при любом зуме)

	curX, curY := screenXA, screenYA

	for _, p := range intersections {
		// Единичный вектор направления отрезка
		dx := screenXB - screenXA
		dy := screenYB - screenYA
		length := math.Sqrt(dx*dx + dy*dy)
		if length == 0 {
			continue
		}
		nx := dx / length
		ny := dy / length

		// Точка перед разрывом
		gapStartX := p.x - nx*gapRadius
		gapStartY := p.y - ny*gapRadius

		// Рисуем сегмент от текущей позиции до разрыва
		vector.StrokeLine(screen,
			float32(curX), float32(curY),
			float32(gapStartX), float32(gapStartY),
			3, c, true)

		// Перескакиваем через разрыв
		curX = p.x + nx*gapRadius
		curY = p.y + ny*gapRadius
	}

	// Финальный сегмент до конца линии
	vector.StrokeLine(screen,
		float32(curX), float32(curY),
		float32(screenXB), float32(screenYB),
		3, c, true)
}

func (g *Game) drawNode(screen *ebiten.Image, node *graph.Node) {
	screenX, screenY := g.camera.WorldToScreen(node.X, node.Y)
	x, y := float32(screenX), float32(screenY)

	// Радиус масштабируется с зумом
	baseRadius := 12.0
	radius := float32(baseRadius * g.camera.Zoom())

	c := data.FactionColors[node.Owner]

	vector.FillCircle(screen, x, y, radius, c, true)
	vector.StrokeCircle(screen, x, y, radius, 2, color.White, true)
}

func (g *Game) drawEdge(screen *ebiten.Image, nodeA, nodeB *graph.Node, edge *graph.Edge) {
	screenXA, screenYA := g.camera.WorldToScreen(nodeA.X, nodeA.Y)
	screenXB, screenYB := g.camera.WorldToScreen(nodeB.X, nodeB.Y)

	// Цвет ребра = цвет линии
	hexColor := g.graph.LineColors[nodeA.LineID]
	c := hexToColor(hexColor)

	if edge.Type == "transfer" {
		c = color.RGBA{255, 255, 255, 150} // Белая пунктирная для пересадок
	}

	g.drawLine(screen, int(screenXA), int(screenYA), int(screenXB), int(screenYB), c, 3)
}

func (g *Game) drawNodeLabel(screen *ebiten.Image, node *graph.Node) {
	if g.fontFace == nil {
		return
	}

	// 1. Базовый размер шрифта (14pt) и текущий зум
	baseFontSize := 14.0
	zoom := g.camera.Zoom()

	// 2. Вычисляем текущий размер шрифта
	currentFontSize := baseFontSize * zoom

	// 3. Получаем шрифт нужного размера (с кэшированием)
	fontFace := g.getFontFace(currentFontSize)

	// 4. Измеряем точный размер текста
	textWidth, textHeight := text.Measure(node.Name, fontFace, 0)

	// 5. Вычисляем размеры прямоугольника
	padding := 6.0 * zoom
	rectWidth := textWidth + 2*padding
	rectHeight := textHeight + 2*padding

	// 6. Позиция прямоугольника (над узлом)
	screenX, screenY := g.camera.WorldToScreen(node.X, node.Y)
	x := float64(screenX)
	y := float64(screenY)
	nodeRadius := 12.0 * zoom
	rectX := x - rectWidth/2
	rectY := y - nodeRadius - 10*zoom - rectHeight

	// 7. Рисуем прямоугольник
	vector.FillRect(screen, float32(rectX), float32(rectY), float32(rectWidth), float32(rectHeight), color.RGBA{0, 0, 0, 200}, true)
	vector.StrokeRect(screen, float32(rectX), float32(rectY), float32(rectWidth), float32(rectHeight), 1, color.RGBA{255, 255, 255, 255}, true)

	// 8. Точное центрирование текста
	textX := rectX + padding
	textY := rectY + padding + (textHeight * 0.5) // Точное вертикальное центрирование

	// 9. Настраиваем опции отрисовки
	op := &text.DrawOptions{}
	op.GeoM.Translate(textX, textY)
	op.ColorScale.ScaleWithColor(color.White)

	// 10. Рисуем текст
	text.Draw(screen, node.Name, fontFace, op)
}

func (g *Game) drawHub(screen *ebiten.Image, hub *graph.Hub) {
	if len(hub.StationIDs) == 0 {
		return
	}

	controller := 0
	allControlled := true
	firstOwner := -1

	for _, stationID := range hub.StationIDs {
		node, exists := g.graph.Nodes[stationID]
		if !exists {
			continue
		}
		if firstOwner == -1 {
			firstOwner = node.Owner
		} else if node.Owner != firstOwner {
			allControlled = false
			break
		}
	}

	if allControlled && firstOwner != 0 {
		controller = firstOwner
	}

	var centerX, centerY float64
	maxDist := 0.0

	for _, stationID := range hub.StationIDs {
		node := g.graph.Nodes[stationID]
		centerX += node.X
		centerY += node.Y
	}
	centerX /= float64(len(hub.StationIDs))
	centerY /= float64(len(hub.StationIDs))

	for _, stationID := range hub.StationIDs {
		node := g.graph.Nodes[stationID]
		dist := (node.X-centerX)*(node.X-centerX) + (node.Y-centerY)*(node.Y-centerY)
		if dist > maxDist {
			maxDist = dist
		}
	}
	radius := int(math.Sqrt(maxDist) + 60)

	screenCenterX, screenCenterY := g.camera.WorldToScreen(centerX, centerY)
	hubRadius := int(float64(radius) * g.camera.Zoom())

	hubColor := color.RGBA{128, 128, 128, 80}
	switch controller {
	case 1:
		hubColor = color.RGBA{200, 50, 50, 80}
	case 2:
		hubColor = color.RGBA{139, 90, 43, 80}
	}

	// Рисуем окружность хаба
	vector.StrokeCircle(screen, float32(screenCenterX), float32(screenCenterY), float32(hubRadius), 3, hubColor, true)
}

func (g *Game) drawLine(screen *ebiten.Image, x0, y0, x1, y1 int, c color.Color, thickness int) {
	// Используем vector для рисования линии
	vector.StrokeLine(screen, float32(x0), float32(y0), float32(x1), float32(y1), float32(thickness), c, true)
}

func (g *Game) drawWorldMarker(screen *ebiten.Image) {
	worldX, worldY := 0.0, 0.0
	screenX, screenY := g.camera.WorldToScreen(worldX, worldY)
	x := int(screenX)
	y := int(screenY)
	baseSize := 20
	size := int(float64(baseSize) * g.camera.Zoom())

	for dy := -size; dy <= size; dy++ {
		if x >= 0 && x < 1920 && (y+dy) >= 0 && (y+dy) < 1080 {
			screen.Set(x, y+dy, color.RGBA{255, 100, 100, 255})
		}
	}
	for dx := -size; dx <= size; dx++ {
		if (x+dx) >= 0 && (x+dx) < 1920 && y >= 0 && y < 1080 {
			screen.Set(x+dx, y, color.RGBA{255, 100, 100, 255})
		}
	}
}

func (g *Game) drawCameraModeIndicator(screen *ebiten.Image) {
	c := color.RGBA{100, 200, 100, 255}
	if g.camera.IsPanning() {
		c = color.RGBA{200, 150, 100, 255}
	}

	for y := 20; y < 50; y++ {
		for x := 20; x < 100; x++ {
			screen.Set(x, y, c)
		}
	}
	for x := 20; x < 100; x++ {
		screen.Set(x, 20, color.White)
		screen.Set(x, 49, color.White)
	}
	for y := 20; y < 50; y++ {
		screen.Set(20, y, color.White)
		screen.Set(99, y, color.White)
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return 1920, 1080
}

// hexToColor конвертирует "#RRGGBB" или "RRGGBB" в color.RGBA
func hexToColor(hex string) color.RGBA {
	// Убираем # если есть
	if len(hex) == 7 && hex[0] == '#' {
		hex = hex[1:]
	}

	if len(hex) != 6 {
		return color.RGBA{200, 200, 200, 255} // Серый по умолчанию
	}

	r := hexToByte(hex[0], hex[1])
	g := hexToByte(hex[2], hex[3])
	b := hexToByte(hex[4], hex[5])
	return color.RGBA{r, g, b, 255}
}

func hexToByte(a, b byte) uint8 {
	return (hexVal(a) << 4) | hexVal(b)
}

func hexVal(c byte) uint8 {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}
