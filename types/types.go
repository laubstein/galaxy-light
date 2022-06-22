package types

type VersionLink struct {
	Version string `json:"version"`
	Href    string `json:"href"`
}

type Versions struct {
	Count    int           `json:"count"`
	Next     string        `json:"next"`
	Previous string        `json:"previous"`
	Results  []VersionLink `json:"results"`
}

type GalaxyFile struct {
	Name           string `json:"name"`
	FType          string `json:"ftype"`
	ChecksumType   string `json:"chksum_type"`
	ChecksumSha256 string `json:"chksum_sha256"`
	Format         int    `json:"format"`
}

type GalaxyFiles struct {
	Files  []GalaxyFile `json:"files"`
	Format int          `json:"format"`
}

type GalaxyCollectionInfo struct {
	Namespace     string            `yaml:"namespace" json:"namespace"`
	Name          string            `yaml:"name" json:"name"`
	Version       string            `yaml:"version" json:"version"`
	Authors       []string          `yaml:"authors" json:"authors"`
	Readme        string            `yaml:"readme" json:"readme"`
	Tags          []string          `yaml:"tags" json:"tags"`
	Description   string            `yaml:"description" json:"description"`
	License       []string          `yaml:"license" json:"license"`
	LicenseFile   string            `yaml:"license_file" json:"license_file"`
	Dependencies  map[string]string `yaml:"dependencies" json:"dependencies"`
	Repository    string            `yaml:"repository" json:"repository"`
	Documentation string            `yaml:"documentation" json:"documentation"`
	Homepage      string            `yaml:"homepage" json:"homepage"`
	Issues        string            `yaml:"issues" json:"issues"`
}

type GalaxyManifest struct {
	CollectionInfo   GalaxyCollectionInfo `yaml:"collection_info" json:"collection_info"`
	FileManifestFile GalaxyFile           `yaml:"file_manifest_file" json:"file_manifest_file"`
	Format           int                  `yaml:"format" json:"format"`
}

type DownloadedFileMetadata struct {
	Dependencies map[string]string `json:"dependencies"`
	Size         int64             `json:"size"`
	Hash         string            `json:"hash"`
}
