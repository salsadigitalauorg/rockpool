package colima

type Cluster struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Cpus   int    `json:"cpus"`
	Memory int    `json:"memory"`
}
