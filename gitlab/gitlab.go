package gitlab

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"time"
)

type GitlabTags struct {
	Name string `json:"name"`
}

func GetTags(gitlabEndpoint, group, namespace, collection string) (out []GitlabTags, status int, err error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/repository/tags", gitlabEndpoint, url.QueryEscape(fmt.Sprintf("%s/%s.%s", group, namespace, collection)))

	req, _ := http.NewRequest("GET", endpoint, nil)

	response, err := client.Do(req)
	if err != nil || response.StatusCode != http.StatusOK {
		if response.StatusCode >= 500 {
			return nil, 500, fmt.Errorf("Failed to load")
		}
		return nil, response.StatusCode, fmt.Errorf("Failed to load")
	}

	decoder := json.NewDecoder(response.Body)
	tags := []GitlabTags{}
	err = decoder.Decode(&tags)
	if err != nil || response.StatusCode != http.StatusOK {
		return nil, 500, fmt.Errorf("Failed to load")
	}

	if len(tags) == 0 {
		return nil, 404, fmt.Errorf("Not found")
	}

	sort.Slice(tags, func(i, j int) bool {
		return tags[i].Name > tags[j].Name
	})

	out = []GitlabTags{}
	for _, v := range tags {
		r, _ := regexp.Compile("^(0|[1-9][0-9]*)[.](0|[1-9][0-9]*)[.](0|[1-9][0-9]*)$")
		if r.MatchString(v.Name) {
			out = append(out, v)
		}
	}

	if len(out) == 0 {
		return nil, 404, fmt.Errorf("Not found")
	}

	return
}
