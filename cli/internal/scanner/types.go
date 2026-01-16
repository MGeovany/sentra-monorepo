package scanner

type EnvFile struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
}

type Project struct {
	RootPath string    `json:"rootPath"`
	EnvFiles []EnvFile `json:"envFiles"`
}
