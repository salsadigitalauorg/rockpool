package lagoon

type Remote struct {
	Id            int    `json:"id"`
	Name          string `json:"name"`
	ConsoleUrl    string `json:"consoleUrl"`
	RouterPattern string `json:"routerPattern"`
}
