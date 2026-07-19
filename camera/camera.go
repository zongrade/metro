package camera

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	MinZoom = 0.3
	MaxZoom = 3.0
	// Множитель скорости панорамирования. Чем больше, тем быстрее двигается карта.
	PanSpeedMultiplier = 1.0
)

type Camera struct {
	// Позиция камеры в мировых координатах (куда смотрит центр экрана)
	x, y float64
	// Текущий масштаб
	zoom float64
	// Размер экрана (логический)
	screenWidth, screenHeight float64

	// Режим панорамирования (toggle по СКМ)
	panning bool
	// Предыдущая позиция мыши для расчёта дельты
	prevMouseX, prevMouseY int
}

func New(screenWidth, screenHeight int) *Camera {
	return &Camera{
		x:            0,
		y:            0,
		zoom:         1.0,
		screenWidth:  float64(screenWidth),
		screenHeight: float64(screenHeight),
		panning:      false,
		prevMouseX:   0,
		prevMouseY:   0,
	}
}

// TogglePanning переключает режим панорамирования
func (c *Camera) TogglePanning() {
	c.panning = !c.panning
	// Сбрасываем предыдущую позицию, чтобы не было "скачка" при входе в режим
	c.prevMouseX, c.prevMouseY = ebiten.CursorPosition()
}

// IsPanning возвращает текущий режим
func (c *Camera) IsPanning() bool {
	return c.panning
}

// Update обрабатывает ввод: зум колесиком и панорамирование
func (c *Camera) Update() {
	c.handleZoom()
	c.handlePanning()
}

// handleZoom обрабатывает прокрутку колесика.
// Зум центрируется на позиции курсора — точка под мышью остаётся на месте.
func (c *Camera) handleZoom() {
	_, dy := ebiten.Wheel()
	if dy == 0 {
		return
	}

	mouseX, mouseY := ebiten.CursorPosition()

	// Мировые координаты точки под курсором ДО зума
	worldX, worldY := c.ScreenToWorld(float64(mouseX), float64(mouseY))

	// Применяем зум (множитель подобран эмпирически для плавности)
	zoomFactor := 1.0 + dy*0.1
	newZoom := c.zoom * zoomFactor

	// Ограничиваем зум
	if newZoom < MinZoom {
		newZoom = MinZoom
	}
	if newZoom > MaxZoom {
		newZoom = MaxZoom
	}
	c.zoom = newZoom

	// Пересчитываем позицию камеры так, чтобы точка под курсором осталась на месте.
	// Формула: screenX = worldX * zoom + camX
	// Отсюда: camX = screenX - worldX * zoom
	c.x = float64(mouseX) - worldX*c.zoom
	c.y = float64(mouseY) - worldY*c.zoom
}

// handlePanning обрабатывает движение мыши в режиме панорамирования
func (c *Camera) handlePanning() {
	if !c.panning {
		c.prevMouseX, c.prevMouseY = ebiten.CursorPosition()
		return
	}

	mouseX, mouseY := ebiten.CursorPosition()
	dx := mouseX - c.prevMouseX
	dy := mouseY - c.prevMouseY

	c.x += float64(dx) * PanSpeedMultiplier
	c.y += float64(dy) * PanSpeedMultiplier

	c.prevMouseX = mouseX
	c.prevMouseY = mouseY
}

// ScreenToWorld преобразует экранные координаты в мировые
func (c *Camera) ScreenToWorld(screenX, screenY float64) (float64, float64) {
	worldX := (screenX - c.x) / c.zoom
	worldY := (screenY - c.y) / c.zoom
	return worldX, worldY
}

// WorldToScreen преобразует мировые координаты в экранные
func (c *Camera) WorldToScreen(worldX, worldY float64) (float64, float64) {
	screenX := worldX*c.zoom + c.x
	screenY := worldY*c.zoom + c.y
	return screenX, screenY
}

// Zoom возвращает текущий масштаб (для использования в отрисовке)
func (c *Camera) Zoom() float64 {
	return c.zoom
}

// X, Y — позиция камеры (для отладки)
func (c *Camera) X() float64 { return c.x }
func (c *Camera) Y() float64 { return c.y }

// ClampZoom ограничивает зум (на случай если где-то задаётся напрямую)
func (c *Camera) ClampZoom() {
	c.zoom = math.Max(MinZoom, math.Min(MaxZoom, c.zoom))
}
