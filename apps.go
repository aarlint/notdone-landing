package main

// AppDef defines an app deployed to the cluster.
type AppDef struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Icon      string `json:"icon"`
	Desc      string `json:"desc"`
	URL       string `json:"url"`
	Repo      string `json:"repo"`
	Category  string `json:"category"`
	Namespace string `json:"-"` // k8s namespace
	Deploy    string `json:"-"` // k8s deployment name
	SvcURL    string `json:"-"` // in-cluster service URL for HTTP probe
}

const (
	CatProductivity = "Productivity"
	CatFitness      = "Fitness & Outdoors"
	CatDevelopment = "Development"
	CatPolitical   = "Political"
	CatFun         = "Fun"
)

// Apps is the canonical list of all deployed applications.
var Apps = []AppDef{
	// Productivity
	{Key: "learn", Name: "Learn", Icon: "book-open", Desc: "Educational games and lessons for kids with profiles and progress tracking", URL: "learn.notdone.dev", Repo: "aarlint/learn", Category: CatProductivity, Namespace: "default", Deploy: "learn", SvcURL: "http://learn.default.svc.cluster.local"},
	{Key: "hunt", Name: "Hunt", Icon: "map", Desc: "Montana hunting app with access maps, regulations, and trip planning", URL: "hunt.notdone.dev", Repo: "aarlint/hunt", Category: CatFitness, Namespace: "default", Deploy: "hunt", SvcURL: "http://hunt.default.svc.cluster.local"},
	{Key: "huddle", Name: "Huddle", Icon: "video", Desc: "Real-time video conferencing with WebRTC, organizations, and multi-room", URL: "huddle.notdone.dev", Repo: "aarlint/huddle", Category: CatProductivity, Namespace: "default", Deploy: "huddle", SvcURL: "http://huddle.default.svc.cluster.local"},
	{Key: "theplan", Name: "The Plan", Icon: "scroll", Desc: "Political reform platform proposing eight policy pillars for governance", URL: "theplan.notdone.dev", Repo: "aarlint/theplan", Category: CatPolitical, Namespace: "default", Deploy: "theplan", SvcURL: "http://theplan.default.svc.cluster.local"},
	{Key: "tools", Name: "Tools", Icon: "wrench", Desc: "Developer utility suite for common tasks and code transformations", URL: "tools.notdone.dev", Repo: "aarlint/tools", Category: CatProductivity, Namespace: "default", Deploy: "tools", SvcURL: "http://tools.default.svc.cluster.local"},
	{Key: "expenses", Name: "Expenses", Icon: "dollar-sign", Desc: "Expense tracker with AI voice input, Claude parsing, and CSV export", URL: "expense-tracker.notdone.dev", Repo: "arlintdev/expenses", Category: CatProductivity, Namespace: "expense-tracker", Deploy: "expense-tracker", SvcURL: "http://expense-tracker.expense-tracker.svc.cluster.local"},

	// Fitness & Outdoors
	{Key: "shredded", Name: "Shredded", Icon: "dumbbell", Desc: "Fitness community for logging lifts, tracking PRs, and achievements", URL: "shredded.notdone.dev", Repo: "aarlint/shredded", Category: CatFitness, Namespace: "default", Deploy: "shredded", SvcURL: "http://shredded.default.svc.cluster.local"},
	{Key: "hikes", Name: "Hikes", Icon: "mountain", Desc: "Community hiking tracker for logging routes, distances, and elevation", URL: "hikes.notdone.dev", Repo: "aarlint/hikemoms", Category: CatFitness, Namespace: "default", Deploy: "hikes", SvcURL: "http://hikes.default.svc.cluster.local"},
	{Key: "plantmontana", Name: "Plant Montana", Icon: "sprout", Desc: "Western Montana gardening guide with frost dates and zone recommendations", URL: "plantmontana.notdone.dev", Repo: "aarlint/plantmontana", Category: CatFitness, Namespace: "default", Deploy: "plantmontana", SvcURL: "http://plantmontana.default.svc.cluster.local"},
	{Key: "fruittrees", Name: "Fruit Trees", Icon: "apple", Desc: "Western fruit tree guide with pollination partners and cold hardiness data", URL: "fruittrees.notdone.dev", Repo: "aarlint/fruittrees", Category: CatFitness, Namespace: "default", Deploy: "fruittrees", SvcURL: "http://fruittrees.default.svc.cluster.local"},

	// Productivity (cont.)
	{Key: "lines", Name: "Lines", Icon: "grid-3x3", Desc: "Montana property lines viewer with parcel search and ownership lookup", URL: "lines.notdone.dev", Repo: "aarlint/lines", Category: CatProductivity, Namespace: "default", Deploy: "lines", SvcURL: "http://lines.default.svc.cluster.local"},
	{Key: "emulator", Name: "Emulator", Icon: "joystick", Desc: "Retro game emulator for N64, SNES, and Genesis with ROM management", URL: "emulator.notdone.dev", Repo: "aarlint/emulator", Category: CatFun, Namespace: "default", Deploy: "emulator", SvcURL: "http://emulator.default.svc.cluster.local"},

	// Development
	{Key: "btdebug", Name: "BT Debug", Icon: "radio", Desc: "Bluetooth diagnostics tool for device scanning and connection monitoring", URL: "btdebug.notdone.dev", Repo: "aarlint/bluetooth", Category: CatDevelopment, Namespace: "default", Deploy: "bluetooth", SvcURL: "http://bluetooth.default.svc.cluster.local"},
	{Key: "code", Name: "Code", Icon: "code", Desc: "Browser-based code editor and development environment", URL: "code.notdone.dev", Repo: "", Category: CatDevelopment, Namespace: "default", Deploy: "code-hub", SvcURL: "http://code-hub.default.svc.cluster.local"},

	// Fun
	{Key: "worlddomination", Name: "World Domination", Icon: "crown", Desc: "Satirical political campaign for a fictional world leader", URL: "worlddomination.notdone.dev", Repo: "aarlint/worlddomination", Category: CatPolitical, Namespace: "default", Deploy: "worlddomination", SvcURL: "http://worlddomination.default.svc.cluster.local"},
}

// CategoryOrder defines display ordering.
var CategoryOrder = []string{CatProductivity, CatFitness, CatDevelopment, CatPolitical, CatFun}
