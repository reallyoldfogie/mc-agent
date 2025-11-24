package models

import "time"

type VersionMetaData struct {
	Arguments              Arguments   `json:"arguments,omitempty"`
	AssetIndex             AssetIndex  `json:"assetIndex,omitempty"`
	Assets                 string      `json:"assets,omitempty"`
	ComplianceLevel        int         `json:"complianceLevel,omitempty"`
	Downloads              Downloads   `json:"downloads,omitempty"`
	ID                     string      `json:"id,omitempty"`
	JavaVersion            JavaVersion `json:"javaVersion,omitempty"`
	Libraries              []Library   `json:"libraries,omitempty"`
	Logging                Logging     `json:"logging,omitempty"`
	MainClass              string      `json:"mainClass,omitempty"`
	MinimumLauncherVersion int         `json:"minimumLauncherVersion,omitempty"`
	ReleaseTime            time.Time   `json:"releaseTime,omitempty"`
	Time                   time.Time   `json:"time,omitempty"`
	Type                   string      `json:"type,omitempty"`
}

type Arguments struct {
	Game []any `json:"arguments,omitempty"`
}

type AssetIndex struct {
	ID string `json:"id,omitempty"`
	Download
	TotalSize int64 `json:"totalSize,omitempty"`
}

type Downloads struct {
	Client         Download `json:"client,omitempty"`
	ClientMappings Download `json:"client_mappings,omitempty"`
	Server         Download `json:"server,omitempty"`
	ServerMappings Download `json:"server_mappings,omitempty"`
}

type Download struct {
	SHA1 string `json:"sha1,omitempty"`
	Size int64  `json:"size,omitempty"`
	URL  string `json:"url,omitempty"`
}

type JavaVersion struct {
	Component    string `json:"component,omitempty"`
	MajorVersion int64  `json:"majorVersion,omitempty"`
}

type Library struct {
	Downloads LibraryDownload `json:"downloads,omitempty"`
	Name      string          `json:"name,omitempty"`
	Rules     []Rule          `json:"rules,omitempty"`
}

type LibraryDownload struct {
	Artifact Artifact `json:"artifact,omitempty"`
}

type Artifact struct {
	Download Download `json:"download,omitempty"`
	Path     string   `json:"path,omitempty"`
}

type Rule struct {
	Action string `json:"action,omitempty"`
	OS     OS     `json:"os,omitempty"`
}

type OS struct {
	Name string `json:"name,omitempty"`
}

type Logging struct {
	Client LoggingClient `json:"client,omitempty"`
}

type LoggingClient struct {
	Argument string       `json:"argument,omitempty"`
	File     FileDownload `json:"file,omitempty"`
}

type FileDownload struct {
	ID string `json:"id,omitempty"`
	Download
	Type string `json:"type,omitempty"`
}
