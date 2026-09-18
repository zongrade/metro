package game

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"image/color"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"metro/camera"
	"metro/cliargs"
	"metro/graph"

	"github.com/erparts/go-shapes"
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
	darkTheme    bool
	labelOffsets map[int]struct{ x, y float64 } // ID узла -> смещение метки
	mapsFS       embed.FS

	// Режим редактирования
	editMode       bool
	draggingNodeID int
	dragOffsetX    float64
	dragOffsetY    float64
	mouseX         float64
	mouseY         float64

	// Работа с картами
	availableMaps []string // Список доступных карт
	currentMap    string   // Текущая карта (без .json)
	showMapMenu   bool     // Показать меню выбора

	shapeRenderer *shapes.Renderer
}

func (g *Game) scanAvailableMaps() error {
	entries, err := g.mapsFS.ReadDir("assets/maps")
	if err != nil {
		return err
	}

	g.availableMaps = []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		// Убираем .json и _ready suffix
		baseName := strings.TrimSuffix(name, ".json")
		baseName = strings.TrimSuffix(baseName, "_ready")

		// Проверяем, что это не _ready версия
		if !strings.Contains(name, "_ready") {
			g.availableMaps = append(g.availableMaps, baseName)
		}
	}

	sort.Strings(g.availableMaps)
	return nil
}

func (g *Game) loadMap(mapName string) error {
	var err error

	// 1. Пробуем загрузить _ready.json из файловой системы (рядом с бинарником)
	readyFile := fmt.Sprintf("assets/maps/%s_ready.json", mapName)
	if _, statErr := os.Stat(readyFile); statErr == nil {
		g.graph, err = graph.LoadMapFromFile(readyFile)
		if err == nil {
			log.Printf("Loaded ready map from file system: %s", readyFile)
			g.finalizeMapLoad(mapName)
			return nil
		}
	}

	// 2. Пробуем загрузить _ready.json из embed FS
	readyEmbed := fmt.Sprintf("assets/maps/%s_ready.json", mapName)
	if _, readErr := g.mapsFS.ReadFile(readyEmbed); readErr == nil {
		g.graph, err = graph.LoadMap(g.mapsFS, readyEmbed)
		if err == nil {
			log.Printf("Loaded ready map from embed: %s", readyEmbed)
			g.finalizeMapLoad(mapName)
			return nil
		}
	}

	// 3. Загружаем обычный .json из embed FS
	filename := fmt.Sprintf("assets/maps/%s.json", mapName)
	g.graph, err = graph.LoadMap(g.mapsFS, filename)
	if err != nil {
		return fmt.Errorf("failed to load map %s: %v", mapName, err)
	}
	log.Printf("Loaded original map from embed: %s", filename)

	g.finalizeMapLoad(mapName)
	return nil
}

// Вынес общую логику в отдельную функцию
func (g *Game) finalizeMapLoad(mapName string) {
	g.labelOffsets = make(map[int]struct{ x, y float64 })
	for id, offset := range g.graph.NodeLabelOffsets {
		g.labelOffsets[id] = struct{ x, y float64 }{x: offset.X, y: offset.Y}
	}

	if len(g.labelOffsets) == 0 {
		g.calculateLabelPositions()
	}

	g.currentMap = mapName
}

func New(mapsFS embed.FS, fontsFS embed.FS) (*Game, error) {
	g := &Game{
		camera:         camera.New(1920, 1080),
		fontsFS:        fontsFS,
		fontCache:      make(map[float64]text.Face),
		mapsFS:         mapsFS,
		baseFontSize:   10.0,
		darkTheme:      true,
		labelOffsets:   make(map[int]struct{ x, y float64 }),
		draggingNodeID: -1,
		showMapMenu:    false,
		shapeRenderer:  shapes.NewRenderer(),
	}

	// Загружаем карту
	var err error
	// Сканируем доступные карты
	if err := g.scanAvailableMaps(); err != nil {
		log.Printf("Warning: Could not scan maps: %v", err)
		g.availableMaps = []string{"Minsk"} // Fallback
	}

	g.currentMap = g.availableMaps[0] // Первая карта по умолчанию
	// Загружаем первую карту
	if err := g.loadMap(g.currentMap); err != nil {
		log.Printf("Warning: Could not load map: %v", err)
		g.graph = graph.NewGraph()
	}

	g.fontFace, err = g.loadFont("assets/fonts/Roboto-Regular.ttf", 14)
	if err != nil {
		log.Printf("Warning: Could not load font: %v", err)
	}

	return g, nil
}

func (g *Game) calculateLabelPositions() {
	type NodeWithIntersections struct {
		nodeID        int
		intersections int
		offset        struct{ x, y float64 }
	}

	// Шаг 1: Вычисляем начальные позиции
	initialOffsets := make(map[int]struct{ x, y float64 })

	for _, node := range g.graph.Nodes {
		neighbors := g.graph.GetNeighbors(node.ID)

		if len(neighbors) == 0 {
			initialOffsets[node.ID] = struct{ x, y float64 }{0, -30}
			continue
		}

		var dirX, dirY float64
		for _, neighborID := range neighbors {
			neighbor := g.graph.Nodes[neighborID]
			dirX += neighbor.X - node.X
			dirY += neighbor.Y - node.Y
		}
		dirX /= float64(len(neighbors))
		dirY /= float64(len(neighbors))

		length := math.Sqrt(dirX*dirX + dirY*dirY)
		if length > 0 {
			dirX /= length
			dirY /= length
		}

		normalX := -dirY
		normalY := dirX
		distance := 25.0
		initialOffsets[node.ID] = struct{ x, y float64 }{normalX * distance, normalY * distance}
	}

	// Шаг 2: Считаем пересечения с учётом реальных размеров
	var nodesWithIntersections []NodeWithIntersections

	for nodeID, offset := range initialOffsets {
		node := g.graph.Nodes[nodeID]
		labelX := node.X + offset.x
		labelY := node.Y + offset.y

		// Получаем реальные размеры метки
		fontFace := g.getFontFace(g.baseFontSize)
		textWidth, textHeight := text.Measure(node.Name, fontFace, 0)
		padding := 2.0
		rectWidth := textWidth + 2*padding
		rectHeight := textHeight + padding

		intersections := 0

		// Проверяем пересечение с другими узлами
		for _, otherNode := range g.graph.Nodes {
			if otherNode.ID == nodeID {
				continue
			}
			dx := labelX - otherNode.X
			dy := labelY - otherNode.Y
			if math.Sqrt(dx*dx+dy*dy) < 15 {
				intersections++
			}
		}

		// Проверяем пересечение с другими метками (с учётом их реальных размеров)
		for otherID, otherOffset := range initialOffsets {
			if otherID == nodeID {
				continue
			}
			otherNode := g.graph.Nodes[otherID]
			otherLabelX := otherNode.X + otherOffset.x
			otherLabelY := otherNode.Y + otherOffset.y

			otherFontFace := g.getFontFace(g.baseFontSize)
			otherTextWidth, otherTextHeight := text.Measure(otherNode.Name, otherFontFace, 0)
			otherPadding := 2.0
			otherRectWidth := otherTextWidth + 2*otherPadding
			otherRectHeight := otherTextHeight + otherPadding

			// Проверяем пересечение прямоугольников
			if g.rectanglesIntersect(
				labelX-rectWidth/2, labelY-rectHeight/2, rectWidth, rectHeight,
				otherLabelX-otherRectWidth/2, otherLabelY-otherRectHeight/2, otherRectWidth, otherRectHeight,
			) {
				intersections++
			}
		}

		nodesWithIntersections = append(nodesWithIntersections, NodeWithIntersections{
			nodeID:        nodeID,
			intersections: intersections,
			offset:        offset,
		})
	}

	sort.Slice(nodesWithIntersections, func(i, j int) bool {
		return nodesWithIntersections[i].intersections > nodesWithIntersections[j].intersections
	})

	// Шаг 3: Размещаем по одной
	g.labelOffsets = make(map[int]struct{ x, y float64 })

	for _, item := range nodesWithIntersections {
		node := g.graph.Nodes[item.nodeID]
		baseOffset := item.offset

		baseAngle := math.Atan2(baseOffset.y, baseOffset.x)
		baseRadius := math.Sqrt(baseOffset.x*baseOffset.x + baseOffset.y*baseOffset.y)

		// Получаем реальные размеры этой метки
		fontFace := g.getFontFace(g.baseFontSize)
		textWidth, textHeight := text.Measure(node.Name, fontFace, 0)
		padding := 2.0
		rectWidth := textWidth + 2*padding
		rectHeight := textHeight + padding

		placed := false

		for radiusMult := 1.0; radiusMult <= 3.0 && !placed; radiusMult += 0.5 {
			currentRadius := baseRadius * radiusMult

			for angle := 0.0; angle < 360 && !placed; angle += 10 {
				rad := baseAngle + angle*math.Pi/180.0

				offset := struct{ x, y float64 }{
					x: currentRadius * math.Cos(rad),
					y: currentRadius * math.Sin(rad),
				}

				if !g.hasIntersectionWithPlaced(node.ID, offset, rectWidth, rectHeight) {
					g.labelOffsets[node.ID] = offset
					placed = true
				}
			}
		}

		if !placed {
			g.labelOffsets[node.ID] = baseOffset
		}
	}
}

func (g *Game) hasIntersectionWithPlaced(nodeID int, offset struct{ x, y float64 }, rectWidth, rectHeight float64) bool {
	node := g.graph.Nodes[nodeID]
	labelX := node.X + offset.x
	labelY := node.Y + offset.y

	// Проверяем пересечение с другими узлами
	for _, otherNode := range g.graph.Nodes {
		if otherNode.ID == nodeID {
			continue
		}
		dx := labelX - otherNode.X
		dy := labelY - otherNode.Y
		if math.Sqrt(dx*dx+dy*dy) < 15 {
			return true
		}
	}

	// Проверяем пересечение с уже размещёнными метками
	for otherID, otherOffset := range g.labelOffsets {
		if otherID == nodeID {
			continue
		}
		otherNode := g.graph.Nodes[otherID]
		otherLabelX := otherNode.X + otherOffset.x
		otherLabelY := otherNode.Y + otherOffset.y

		// Получаем размеры другой метки
		fontFace := g.getFontFace(g.baseFontSize)
		otherTextWidth, otherTextHeight := text.Measure(otherNode.Name, fontFace, 0)
		otherPadding := 2.0
		otherRectWidth := otherTextWidth + 2*otherPadding
		otherRectHeight := otherTextHeight + otherPadding

		if g.rectanglesIntersect(
			labelX-rectWidth/2, labelY-rectHeight/2, rectWidth, rectHeight,
			otherLabelX-otherRectWidth/2, otherLabelY-otherRectHeight/2, otherRectWidth, otherRectHeight,
		) {
			return true
		}
	}

	return false
}

// rectanglesIntersect проверяет пересечение двух прямоугольников
func (g *Game) rectanglesIntersect(x1, y1, w1, h1, x2, y2, w2, h2 float64) bool {
	return !(x1+w1 < x2 || x2+w2 < x1 || y1+h1 < y2 || y2+h2 < y1)
}

func (g *Game) hasIntersection(nodeID int, offset struct{ x, y float64 }) bool {
	node := g.graph.Nodes[nodeID]
	labelX := node.X + offset.x
	labelY := node.Y + offset.y

	labelWidth := 100.0
	labelHeight := 20.0

	// Проверяем пересечение с другими узлами
	for _, otherNode := range g.graph.Nodes {
		if otherNode.ID == nodeID {
			continue
		}

		dx := labelX - otherNode.X
		dy := labelY - otherNode.Y
		distance := math.Sqrt(dx*dx + dy*dy)

		if distance < 15 { // Радиус узла + запас
			return true
		}
	}

	// Проверяем пересечение с другими метками
	for otherID, otherOffset := range g.labelOffsets {
		if otherID == nodeID {
			continue
		}

		otherNode := g.graph.Nodes[otherID]
		otherLabelX := otherNode.X + otherOffset.x
		otherLabelY := otherNode.Y + otherOffset.y

		// Проверяем пересечение прямоугольников
		if math.Abs(labelX-otherLabelX) < labelWidth && math.Abs(labelY-otherLabelY) < labelHeight {
			return true
		}
	}

	return false
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

	mx, my := ebiten.CursorPosition()
	g.mouseX, g.mouseY = float64(mx), float64(my)

	// Toggle map menu: M
	if inpututil.IsKeyJustPressed(ebiten.KeyM) {
		g.showMapMenu = !g.showMapMenu
	}

	// Toggle edit mode: Shift+E
	if inpututil.IsKeyJustPressed(ebiten.KeyE) && ebiten.IsKeyPressed(ebiten.KeyShift) {
		g.editMode = !g.editMode
		g.draggingNodeID = -1
	}

	// Save: Shift+S
	if inpututil.IsKeyJustPressed(ebiten.KeyS) && ebiten.IsKeyPressed(ebiten.KeyShift) {
		g.saveMap(fmt.Sprintf("assets/maps/%s_ready.json", g.currentMap))
	}

	// Recalculate: Shift+R
	if inpututil.IsKeyJustPressed(ebiten.KeyR) && ebiten.IsKeyPressed(ebiten.KeyShift) {
		g.calculateLabelPositions()
	}

	// Reset label: Delete
	if inpututil.IsKeyJustPressed(ebiten.KeyDelete) && g.editMode {
		g.resetHoveredLabel()
	}

	// Обработка клика по кнопке темы
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		bufx, bufy := ebiten.CursorPosition()
		mx, my := float64(bufx), float64(bufy)

		// Координаты должны совпадать с drawThemeToggleButton
		buttonX := 1920. - 30
		buttonY := 35.

		// Размер кнопки = размер текста
		textWidth, textHeight := text.Measure("Light", g.fontFace, 0)
		padding := 10.0
		buttonW := textWidth + padding*2
		buttonH := textHeight + padding*2

		if mx >= buttonX-buttonW/2 && mx <= buttonX+buttonW/2 &&
			my >= buttonY-buttonH/2 && my <= buttonY+buttonH/2 {
			g.darkTheme = !g.darkTheme
		}

		if mx > 19 && mx < 110 && my > 52 && my < 87 {

			g.showMapMenu = !g.showMapMenu
		}

		// выбор карты
		{
			menuX := 20.0
			menuY := 90.0
			if g.editMode {
				menuY = 130.0
			}

			itemHeight := 30.0
			gap := 4.0
			totalItemHeight := itemHeight + gap
			menuWidth := 150.0

			for i, mapName := range g.availableMaps {
				itemY := menuY + 40.0 + float64(i)*totalItemHeight

				// Та же логика, что и в drawMapMenu
				if g.mouseX >= menuX && g.mouseX <= menuX+menuWidth &&
					g.mouseY >= itemY && g.mouseY < itemY+itemHeight {

					if err := g.loadMap(mapName); err != nil {
						log.Printf("Error loading map: %v", err)
					}
					g.showMapMenu = false
					break // Выходим из цикла после успешного выбора
				}
			}

		}
	}

	if g.editMode {
		g.handleEditModeInput()
	}

	g.camera.Update()

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return ebiten.Termination
	}

	return nil
}

func (g *Game) drawUIBox(screen *ebiten.Image, textStr string, centerX, centerY float64, bgColor, textColor, borderColor color.Color) {
	// 1. Точно измеряем текст
	tw, th := text.Measure(textStr, g.fontFace, 0)

	// 2. Фиксированные отступы (в экранных пикселях, чтобы UI не прыгал при зуме)
	padding := 10.0
	rectW := tw + padding*2
	rectH := th + padding*2

	// 3. Центрируем прямоугольник относительно переданных координат
	rectX := centerX - rectW/2
	rectY := centerY - rectH/2

	outerRounding := float32(6.0)
	thickness := float32(1.0)
	innerRounding := outerRounding - thickness

	// 1. Внешняя граница
	borderRGBA, ok := borderColor.(color.RGBA)
	if ok && borderRGBA.A > 0 {
		g.shapeRenderer.SetColor(borderColor)
		g.shapeRenderer.DrawArea(screen, float32(rectX), float32(rectY), float32(rectW), float32(rectH), outerRounding)
	}

	// 2. Внутренний фон
	bgRGBA, ok := bgColor.(color.RGBA)
	if ok && bgRGBA.A > 0 {
		g.shapeRenderer.SetColor(bgColor)
		g.shapeRenderer.DrawArea(screen,
			float32(rectX)+thickness,
			float32(rectY)+thickness,
			float32(rectW)-2*thickness,
			float32(rectH)-2*thickness,
			innerRounding)
	}

	// 5. Позиция текста (с учётом baseline шрифта)
	textX := rectX + padding
	textY := rectY + padding

	op := &text.DrawOptions{}
	op.GeoM.Translate(textX, textY)
	op.ColorScale.ScaleWithColor(textColor)
	text.Draw(screen, textStr, g.fontFace, op)
}

func (g *Game) drawMapMenu(screen *ebiten.Image) {
	if !g.showMapMenu {
		return
	}

	menuX := 20.0
	menuY := 90.0
	if g.editMode {
		menuY = 130.
	}

	itemHeight := 30.0
	gap := 4.0
	totalItemHeight := itemHeight + gap

	menuWidth := 150.0
	menuHeight := float64(len(g.availableMaps))*totalItemHeight + 40

	// Фон всего меню
	if g.darkTheme {
		vector.FillRect(screen, float32(menuX), float32(menuY), float32(menuWidth), float32(menuHeight), color.RGBA{50, 50, 50, 100}, true)
	} else {
		vector.FillRect(screen, float32(menuX), float32(menuY), float32(menuWidth), float32(menuHeight), color.RGBA{200, 200, 200, 100}, true)
	}
	// Граница меню
	if g.darkTheme {
		vector.StrokeRect(screen, float32(menuX), float32(menuY), float32(menuWidth), float32(menuHeight), 2, color.RGBA{255, 255, 0, 255}, true)
	} else {
		vector.StrokeRect(screen, float32(menuX), float32(menuY), float32(menuWidth), float32(menuHeight), 2, color.RGBA{96, 124, 196, 255}, true)
	}

	// Заголовок
	op := &text.DrawOptions{}
	op.GeoM.Translate(menuX+10, menuY+10)
	if g.darkTheme {
		op.ColorScale.ScaleWithColor(color.RGBA{255, 255, 0, 255})
	} else {
		op.ColorScale.ScaleWithColor(color.RGBA{96, 124, 196, 255})
	}
	text.Draw(screen, "Select Map (Click):", g.fontFace, op)

	// Список карт
	for i, mapName := range g.availableMaps {
		itemY := menuY + 30 + float64(i)*totalItemHeight

		// Проверяем наведение мыши
		isHovered := g.mouseX >= menuX && g.mouseX <= menuX+menuWidth &&
			g.mouseY >= itemY && g.mouseY <= itemY+itemHeight

		// Подсветка при наведении
		if isHovered {
			if g.darkTheme {
				vector.FillRect(screen, float32(menuX+5), float32(itemY), float32(menuWidth-10), float32(itemHeight), color.RGBA{100, 100, 0, 10}, true)
			} else {
				vector.FillRect(screen, float32(menuX+5), float32(itemY), float32(menuWidth-10), float32(itemHeight), color.RGBA{96, 124, 196, 150}, true)
			}
		}

		// Текст элемента
		label := fmt.Sprintf("  %s", mapName)
		if g.currentMap == mapName {
			label = fmt.Sprintf("► %s", mapName)
		}

		op := &text.DrawOptions{}
		//op.GeoM.Translate(menuX+10, itemY+20) // +20 для вертикального центрирования в itemHeight=30
		op.GeoM.Translate(menuX+10, itemY+7) // +20 для вертикального центрирования в itemHeight=30

		if g.currentMap == mapName {
			if g.darkTheme {
				op.ColorScale.ScaleWithColor(color.RGBA{255, 255, 0, 255})
			} else {
				op.ColorScale.ScaleWithColor(color.RGBA{96, 124, 196, 255})
			}
		} else if isHovered {
			op.ColorScale.ScaleWithColor(color.RGBA{0, 150, 0, 255})
		} else {
			op.ColorScale.ScaleWithColor(color.RGBA{200, 200, 200, 255})
		}

		text.Draw(screen, label, g.fontFace, op)
	}
}

func (g *Game) handleEditModeInput() {
	// Находим метку под курсором
	hoveredNodeID := g.findHoveredLabel()

	if g.draggingNodeID != -1 {
		// Перетаскивание
		if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
			screenX, screenY := g.camera.WorldToScreen(g.graph.Nodes[g.draggingNodeID].X, g.graph.Nodes[g.draggingNodeID].Y)

			offset := g.labelOffsets[g.draggingNodeID]
			labelScreenX := screenX + offset.x*g.camera.Zoom()
			labelScreenY := screenY + offset.y*g.camera.Zoom()

			dx := (g.mouseX - labelScreenX) / g.camera.Zoom()
			dy := (g.mouseY - labelScreenY) / g.camera.Zoom()

			offset.x += dx
			offset.y += dy
			g.labelOffsets[g.draggingNodeID] = offset
		} else {
			// Отпустили мышь
			g.draggingNodeID = -1
		}
	} else if hoveredNodeID != -1 {
		// Начинаем перетаскивание
		if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
			g.draggingNodeID = hoveredNodeID
			screenX, screenY := g.camera.WorldToScreen(g.graph.Nodes[hoveredNodeID].X, g.graph.Nodes[hoveredNodeID].Y)

			offset := g.labelOffsets[hoveredNodeID]
			labelScreenX := screenX + offset.x*g.camera.Zoom()
			labelScreenY := screenY + offset.y*g.camera.Zoom()

			g.dragOffsetX = g.mouseX - labelScreenX
			g.dragOffsetY = g.mouseY - labelScreenY
		}
	}
}

func (g *Game) findHoveredLabel() int {
	for _, node := range g.graph.Nodes {
		offset, exists := g.labelOffsets[node.ID]
		if !exists {
			continue
		}

		screenX, screenY := g.camera.WorldToScreen(node.X, node.Y)
		labelX := screenX + offset.x*g.camera.Zoom()
		labelY := screenY + offset.y*g.camera.Zoom()

		fontFace := g.getFontFace(g.baseFontSize * g.camera.Zoom())
		textWidth, textHeight := text.Measure(node.Name, fontFace, 0)
		padding := 2.0 * g.camera.Zoom()
		rectWidth := textWidth + 2*padding
		rectHeight := textHeight + padding

		rectX := labelX - rectWidth/2
		rectY := labelY - rectHeight/2

		if g.mouseX >= rectX && g.mouseX <= rectX+rectWidth &&
			g.mouseY >= rectY && g.mouseY <= rectY+rectHeight {
			return node.ID
		}
	}
	return -1
}

func (g *Game) resetHoveredLabel() {
	hoveredID := g.findHoveredLabel()
	if hoveredID != -1 {
		delete(g.labelOffsets, hoveredID)
		// Пересчитываем позицию для этого узла
		neighbors := g.graph.GetNeighbors(hoveredID)
		if len(neighbors) > 0 {
			node := g.graph.Nodes[hoveredID]
			var dirX, dirY float64
			for _, neighborID := range neighbors {
				neighbor := g.graph.Nodes[neighborID]
				dirX += neighbor.X - node.X
				dirY += neighbor.Y - node.Y
			}
			dirX /= float64(len(neighbors))
			dirY /= float64(len(neighbors))

			length := math.Sqrt(dirX*dirX + dirY*dirY)
			if length > 0 {
				dirX /= length
				dirY /= length
			}

			normalX := -dirY
			normalY := dirX
			g.labelOffsets[hoveredID] = struct{ x, y float64 }{normalX * 25.0, normalY * 25.0}
		}
	}
}

func (g *Game) saveMap(filename string) {
	mapData := graph.MapData{
		Nodes:      make([]graph.NodeJSON, 0, len(g.graph.Nodes)),
		Edges:      make([]graph.EdgeJSON, 0, len(g.graph.Edges)),
		Hubs:       make([]graph.HubJSON, 0, len(g.graph.Hubs)),
		LineColors: g.graph.LineColors,
	}

	// Сохраняем узлы с label_offset
	for _, node := range g.graph.Nodes {
		nodeWithOffset := graph.NodeJSON{
			ID:     node.ID,
			Name:   node.Name,
			X:      node.X,
			Y:      node.Y,
			LineID: node.LineID,
			Type:   int(node.Type),
			Owner:  node.Owner,
		}

		if offset, exists := g.labelOffsets[node.ID]; exists {
			nodeWithOffset.LabelOffset = &graph.LabelOffset{
				X: offset.x,
				Y: offset.y,
			}
		}

		mapData.Nodes = append(mapData.Nodes, nodeWithOffset)
	}

	// Сохраняем рёбра
	for _, edge := range g.graph.Edges {
		mapData.Edges = append(mapData.Edges, graph.EdgeJSON{
			ID:     edge.ID,
			From:   edge.From,
			To:     edge.To,
			Length: edge.Length,
			Type:   edge.Type,
		})
	}

	// Сохраняем хабы
	for _, hub := range g.graph.Hubs {
		mapData.Hubs = append(mapData.Hubs, graph.HubJSON{
			ID:         hub.ID,
			Name:       hub.Name,
			StationIDs: hub.StationIDs,
		})
	}

	// Маршалим в JSON
	output, err := json.MarshalIndent(mapData, "", "  ")
	if err != nil {
		log.Printf("Error marshaling map: %v", err)
		return
	}

	// 1. Создаём директорию, если её нет
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("Error creating directory %s: %v", dir, err)
		return
	}

	// Записываем в файл
	err = os.WriteFile(filename, output, 0644)
	if err != nil {
		log.Printf("Error saving map: %v", err)
		return
	}

	log.Printf("Map saved to %s", filename)
}

func (g *Game) Draw(screen *ebiten.Image) {
	if g.darkTheme {
		screen.Fill(color.RGBA{20, 20, 25, 255})
	} else {
		screen.Fill(color.RGBA{240, 240, 245, 255})
	}

	if len(g.graph.Nodes) > 0 {
		g.drawGraph(screen)
	} else {
		g.drawWorldMarker(screen)
	}

	g.drawCameraModeIndicator(screen)
	g.drawThemeToggleButton(screen)
	g.drawMapNameIndicator(screen)

	if g.editMode {
		g.drawEditModeIndicator(screen)
		g.drawConflictCounter(screen)
	}

	g.drawMapMenu(screen)
	g.drawDebugInfo(screen)
}

func (g *Game) drawMapNameIndicator(screen *ebiten.Image) {
	textStr := fmt.Sprintf("Map: %s", g.currentMap)
	bgColor := color.RGBA{0, 0, 0, 180}
	textColor := color.RGBA{200, 200, 200, 255}

	g.drawUIBox(screen, textStr, 65, 70, bgColor, textColor, textColor)
}

func (g *Game) drawEditModeIndicator(screen *ebiten.Image) {
	bgColor := color.RGBA{255, 200, 0, 200}
	textColor := color.Black
	borderColor := color.RGBA{255, 255, 0, 255}
	g.drawUIBox(screen, "EDIT MODE", 65, 110, bgColor, textColor, borderColor)
}

func (g *Game) drawDebugInfo(screen *ebiten.Image) {
	if !cliargs.DEBUG {
		return
	}

	debugText := fmt.Sprintf("Cursor: X:%.0f Y:%.0f", g.mouseX, g.mouseY)

	// Рисуем в правом нижнем углу или рядом с другими индикаторами
	bgColor := color.RGBA{0, 0, 0, 180}
	textColor := color.RGBA{0, 255, 0, 255} // Зелёный для отладки
	borderColor := color.RGBA{0, 255, 0, 255}

	g.drawUIBox(screen, debugText, 1920-120, 80, bgColor, textColor, borderColor)
}

func (g *Game) drawConflictCounter(screen *ebiten.Image) {
	conflicts := g.countConflicts()
	textStr := fmt.Sprintf("Conflicts: %d", conflicts)

	var bgColor, textColor, borderColor color.Color

	if conflicts == 0 {
		// Мягкий, приятный для глаз зеленый
		bgColor = color.RGBA{70, 150, 70, 220}
		textColor = color.White
		borderColor = color.RGBA{50, 130, 50, 255}
	} else {
		// Мягкий, но заметный красный
		bgColor = color.RGBA{180, 70, 70, 220}
		textColor = color.White
		borderColor = color.RGBA{160, 50, 50, 255}
	}

	g.drawUIBox(screen, textStr, 1920-120, 35, bgColor, textColor, borderColor)
}

func (g *Game) countConflicts() int {
	conflicts := 0
	for nodeID := range g.labelOffsets {
		if g.hasLabelConflict(nodeID) {
			conflicts++
		}
	}
	return conflicts
}

func (g *Game) hasLabelConflict(nodeID int) bool {
	node := g.graph.Nodes[nodeID]
	offset := g.labelOffsets[nodeID]

	labelX := node.X + offset.x
	labelY := node.Y + offset.y

	fontFace := g.getFontFace(g.baseFontSize)
	textWidth, textHeight := text.Measure(node.Name, fontFace, 0)
	padding := 2.0
	rectWidth := textWidth + 2*padding
	rectHeight := textHeight + padding

	// Проверяем пересечение с узлами
	for _, otherNode := range g.graph.Nodes {
		if otherNode.ID == nodeID {
			continue
		}
		dx := labelX - otherNode.X
		dy := labelY - otherNode.Y
		if math.Sqrt(dx*dx+dy*dy) < 15 {
			return true
		}
	}

	// Проверяем пересечение с другими метками
	for otherID, otherOffset := range g.labelOffsets {
		if otherID == nodeID {
			continue
		}
		otherNode := g.graph.Nodes[otherID]
		otherLabelX := otherNode.X + otherOffset.x
		otherLabelY := otherNode.Y + otherOffset.y

		otherFontFace := g.getFontFace(g.baseFontSize)
		otherTextWidth, otherTextHeight := text.Measure(otherNode.Name, otherFontFace, 0)
		otherPadding := 2.0
		otherRectWidth := otherTextWidth + 2*otherPadding
		otherRectHeight := otherTextHeight + otherPadding

		if g.rectanglesIntersect(
			labelX-rectWidth/2, labelY-rectHeight/2, rectWidth, rectHeight,
			otherLabelX-otherRectWidth/2, otherLabelY-otherRectHeight/2, otherRectWidth, otherRectHeight,
		) {
			return true
		}
	}

	return false
}

func (g *Game) drawThemeToggleButton(screen *ebiten.Image) {
	textStr := "Light"
	if g.darkTheme {
		textStr = "Dark"
	}

	// Явно объявляем переменные как color.Color
	var bgColor, textColor, borderColor color.Color

	if g.darkTheme {
		bgColor = color.RGBA{50, 50, 50, 100}
		textColor = color.White
		borderColor = color.White
	} else {
		bgColor = color.RGBA{200, 200, 200, 100}     // Полупрозрачный фон
		textColor = color.Black                      // Черный текст
		borderColor = color.RGBA{128, 128, 128, 255} // Серая обводка
	}

	g.drawUIBox(screen, textStr, 1920-30, 35, bgColor, textColor, borderColor)
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

	baseRadius := 6.0
	radius := float32(baseRadius * g.camera.Zoom())

	// Заливка узла цветом линии
	lineColor := hexToColor(g.graph.LineColors[node.LineID])
	vector.FillCircle(screen, x, y, radius, color.White, true)

	// Чёрная внешняя обводка для контраста
	vector.StrokeCircle(screen, x, y, radius, 2, color.Black, true)

	// Белая точка в центре
	centerDotRadius := float32(4.0 * g.camera.Zoom())
	vector.FillCircle(screen, x, y, centerDotRadius, lineColor, true)
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

	offset, exists := g.labelOffsets[node.ID]
	if !exists {
		return
	}

	zoom := g.camera.Zoom()
	currentFontSize := g.baseFontSize * zoom
	fontFace := g.getFontFace(currentFontSize)

	textWidth, textHeight := text.Measure(node.Name, fontFace, 0)

	padding := 2.0 * zoom
	rectWidth := textWidth + 2*padding
	rectHeight := textHeight + padding // Твоё упрощение

	screenX, screenY := g.camera.WorldToScreen(node.X, node.Y)
	x := float64(screenX)
	y := float64(screenY)

	// Координаты прямоугольника подписи
	rectX := x + offset.x*zoom - rectWidth/2
	rectY := y + offset.y*zoom - rectHeight/2

	lineColor := hexToColor(g.graph.LineColors[node.LineID])

	// Определяем цвета
	hasConflict := g.editMode && g.hasLabelConflict(node.ID)
	isDragging := g.draggingNodeID == node.ID

	var bgColor, borderColor, textColor color.Color

	if g.darkTheme {
		//bgColor = color.RGBA{0, 0, 0, 0}
		bgColor = color.RGBA{20, 20, 25, 255}
		borderColor = lineColor
		textColor = color.White
	} else {
		//bgColor = color.RGBA{240, 240, 245, 0} // Полупрозрачный белый
		bgColor = color.RGBA{240, 240, 245, 255} // Полупрозрачный белый
		borderColor = lineColor
		textColor = color.Black // Черный текст
	}

	if hasConflict {
		borderColor = color.RGBA{255, 0, 0, 255}
	}
	if isDragging {
		borderColor = color.RGBA{255, 255, 0, 255}
	}

	// Рисуем прямоугольник со скруглёнными углами через go-shapes
	if g.editMode {
		// В режиме редактирования пока оставим пунктирную рамку (или можно применить тот же трюк)
		g.drawDashedRect(screen, float32(rectX), float32(rectY), float32(rectWidth), float32(rectHeight), 4, borderColor)
	} else {
		thickness := float32(1.0)
		outerRounding := float32(5.0) * float32(zoom)
		innerRounding := outerRounding - thickness
		if innerRounding < 0 {
			innerRounding = 0
		}

		// 1. Рисуем внешнюю границу (если её альфа > 0)
		borderRGBA, ok := borderColor.(color.RGBA)
		if ok && borderRGBA.A > 0 {
			g.shapeRenderer.SetColor(borderColor)
			g.shapeRenderer.DrawArea(screen, float32(rectX), float32(rectY), float32(rectWidth), float32(rectHeight), outerRounding)
		}

		// 2. Рисуем внутренний фон (если его альфа > 0)
		bgRGBA, ok := bgColor.(color.RGBA)
		if ok && bgRGBA.A > 0 {
			g.shapeRenderer.SetColor(bgColor)
			// Внутренние координаты смещены на thickness, размеры уменьшены на 2*thickness
			g.shapeRenderer.DrawArea(screen,
				float32(rectX)+thickness,
				float32(rectY)+thickness,
				float32(rectWidth)-2*thickness,
				float32(rectHeight)-2*thickness,
				innerRounding)
		}
	}

	// Рисуем текст
	textX := rectX + padding
	textY := rectY + padding

	op := &text.DrawOptions{}
	op.GeoM.Translate(textX, textY)
	op.ColorScale.ScaleWithColor(textColor)
	text.Draw(screen, node.Name, fontFace, op)

	// ==========================================================
	// Рисуем линию связи от узла к подписи
	// ==========================================================

	// 1. Находим ближайшую точку на прямоугольнике подписи к центру узла
	// (Используем clamp: ограничиваем координаты центра узла границами прямоугольника)
	closestX := math.Max(float64(rectX), math.Min(x, float64(rectX+rectWidth)))
	closestY := math.Max(float64(rectY), math.Min(y, float64(rectY+rectHeight)))

	// 2. Вычисляем вектор от центра узла к этой ближайшей точке
	dx := closestX - x
	dy := closestY - y
	dist := math.Sqrt(dx*dx + dy*dy)

	// 3. Если расстояние больше 0, рисуем линию от КРАЯ узла (а не от центра)
	if dist > 0 {
		nodeRadius := 12.0 * zoom
		startX := x + (dx/dist)*nodeRadius
		startY := y + (dy/dist)*nodeRadius

		// Рисуем линию связи тем же цветом, что и граница
		lineColor := hexToColor(g.graph.LineColors[node.LineID])
		vector.StrokeLine(screen, float32(startX), float32(startY), float32(closestX), float32(closestY), 1.5, lineColor, true)
	}
}

func (g *Game) drawDashedRect(screen *ebiten.Image, x, y, w, h float32, dashLength int, c color.Color) {
	// Рисуем пунктирную рамку
	// Верхняя линия
	for i := 0; i < int(w); i += dashLength * 2 {
		length := math.Min(float64(dashLength), float64(int(w)-i))
		vector.StrokeLine(screen, x+float32(i), y, x+float32(i)+float32(length), y, 1, c, true)
	}
	// Правая линия
	for i := 0; i < int(h); i += dashLength * 2 {
		length := math.Min(float64(dashLength), float64(int(h)-i))
		vector.StrokeLine(screen, x+w, y+float32(i), x+w, y+float32(i)+float32(length), 1, c, true)
	}
	// Нижняя линия
	for i := 0; i < int(w); i += dashLength * 2 {
		length := math.Min(float64(dashLength), float64(int(w)-i))
		vector.StrokeLine(screen, x+float32(i), y+h, x+float32(i)+float32(length), y+h, 1, c, true)
	}
	// Левая линия
	for i := 0; i < int(h); i += dashLength * 2 {
		length := math.Min(float64(dashLength), float64(int(h)-i))
		vector.StrokeLine(screen, x, y+float32(i), x, y+float32(i)+float32(length), 1, c, true)
	}
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
