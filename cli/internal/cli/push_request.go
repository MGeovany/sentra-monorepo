package cli

type pushRequestV1 struct {
	V       int           `json:"v"`
	Project pushProjectV1 `json:"project"`
	Machine pushMachineV1 `json:"machine"`
	Commit  pushCommitV1  `json:"commit"`
	Files   []pushFileV1  `json:"files"`
}

type pushProjectV1 struct {
	Root string `json:"root,omitempty"`
	ID   string `json:"id,omitempty"`
}

type pushMachineV1 struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

type pushCommitV1 struct {
	ClientID       string `json:"client_id"`
	Message        string `json:"message"`
	ParentClientID string `json:"parent_client_id,omitempty"`
}

type pushFileV1 struct {
	Path      string         `json:"path"`
	SHA256    string         `json:"sha256"`
	Size      int            `json:"size"`
	Encrypted bool           `json:"encrypted"`
	Cipher    string         `json:"cipher"`
	Blob      string         `json:"blob,omitempty"`
	Storage   *pushStorageV1 `json:"storage,omitempty"`
}

type pushStorageV1 struct {
	Provider string `json:"provider"`
	Bucket   string `json:"bucket"`
	Key      string `json:"key"`
	Endpoint string `json:"endpoint,omitempty"`
	Region   string `json:"region,omitempty"`
}
