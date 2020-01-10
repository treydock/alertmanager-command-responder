package version

import "encoding/json"

var (
	gitTag    string
	gitSha    string
	buildTime string
)

type (
	Version struct {
		GitTag    string `json:"version"`
		GitSha    string `json:"sha1sum"`
		BuildTime string `json:"buildtime"`
	}
)

func VersionJSON() []byte {
	versionValue := Version{gitTag, gitSha, buildTime}
	jsonValue, _ := json.Marshal(versionValue)
	return jsonValue
}
