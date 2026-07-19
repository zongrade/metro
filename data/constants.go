package data

// Типы ресурсов
const (
	ResourceProvision = "provision" // Провизия (еда)
	ResourceMetal     = "metal"     // Металлолом
	ResourceParts     = "parts"     // Детали (НИОКР)
)

// Типы узлов
type NodeType int

const (
	NodeResource NodeType = iota
	NodeProduction
	NodeCapital
)

// Типы пересадочных узлов
type HubType int

const (
	HubMixed HubType = iota
	HubCapital
	HubProduction
	HubResource
)

// Уровни развития
const (
	MaxLevel = 5
)

// Базовый доход (до множителя уровня)
var BaseIncome = map[NodeType]map[string]float64{
	NodeResource: {
		ResourceProvision: 8,
		ResourceMetal:     0,
		ResourceParts:     0,
	},
	NodeProduction: {
		ResourceProvision: 3,
		ResourceMetal:     2,
		ResourceParts:     2,
	},
	NodeCapital: {
		ResourceProvision: 2,
		ResourceMetal:     1,
		ResourceParts:     5,
	},
}

// Множитель дохода по уровням (логарифмический рост)
var LevelMultiplier = map[int]float64{
	1: 1.0,
	2: 1.3,
	3: 1.6,
	4: 2.0,
	5: 2.5,
}

// Затраты Провизии на содержание узла
var LevelUpkeep = map[int]float64{
	1: 0,
	2: 1,
	3: 3,
	4: 6,
	5: 10,
}

// Стоимость апгрейда (Металл, Детали, Ходы)
var UpgradeCost = map[int]struct {
	Metal int
	Parts int
	Turns int
}{
	1: {10, 5, 3},
	2: {20, 10, 5},
	3: {40, 20, 8},
	4: {80, 40, 12},
}

// Бонусы кластеров
const (
	HubCapitalBonusPerStation    = 0.15 // +15% к деталям за станцию
	HubProductionBonusPerStation = 0.25 // +25% к производству за станцию
	HubResourceBonusPerStation   = 0.20 // +20% к добыче за станцию
	HubCapitalAdminPerStation    = 1    // +1 ОД за станцию в столичном кластере
)

// Лимиты
const (
	MaxArmySize    = 1000 // Максимальный размер армии
	FrontlineLimit = 150  // Лимит фронта в тоннеле
	TooltipDelay   = 400  // Задержка тултипа в мс
)
