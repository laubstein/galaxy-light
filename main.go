package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing/fstest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/laubstein/galaxy-light/gitlab"
	"github.com/laubstein/galaxy-light/types"
	"github.com/laubstein/galaxy-light/util"
	"gopkg.in/yaml.v2"
)

var (
	GITLAB_ENDPOINT   = util.GetEnv("GALAXY_LIGHT_GITLAB_ENDPOINT", "http://127.0.0.1:8080")
	GITLAB_ROOT_GROUP = util.GetEnv("GALAXY_LIGHT_GITLAB_ROOT_GROUP", "ansible/collections")
	TARGET_PATH       = util.GetEnv("GALAXY_LIGHT_TARGET_PATH", "/tmp/galaxy-light")
	SERVER_BIND       = util.GetEnv("GALAXY_LIGHT_SERVER_BIND", "127.0.0.1")
	SERVER_PORT       = util.GetEnv("GALAXY_LIGHT_SERVER_PORT", "8181")
	SERVER_PROTOCOL   = util.GetEnv("GALAXY_LIGHT_SERVER_PROTOCOL", "http")
)

func DownloadGitlabRepositoryAsGalaxyPackage(version, targetPath, endpoint string) (result types.DownloadedFileMetadata, err error) {
	var dependencies map[string]string
	var size int64

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	req, err := http.NewRequest("GET", endpoint, nil)

	if err != nil {
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		return
	}

	defer resp.Body.Close()

	repositoryFiles, err := util.TarGzMemory(resp.Body)

	repositoryRootPath, err := repositoryFiles.Glob("*")
	if err != nil || len(repositoryRootPath) != 1 {
		if err == nil {
			err = fmt.Errorf("Failed to list tar.gz root dir name. " + endpoint)
		}
		return
	}

	var tarGzBuffer bytes.Buffer
	zr := gzip.NewWriter(&tarGzBuffer)
	tw := tar.NewWriter(zr)

	galaxyFiles := types.GalaxyFiles{
		Files:  []types.GalaxyFile{},
		Format: 1,
	}

	fs.WalkDir(repositoryFiles, repositoryRootPath[0], func(currentPath string, d fs.DirEntry, err error) error {
		if repositoryRootPath[0] == currentPath {
			return nil
		}

		currentPathStat, _ := repositoryFiles.Stat(currentPath)
		finalName := currentPath[strings.Index(currentPath, "/")+1:]
		var header *tar.Header
		if currentPath == d.Name() {
			// fix mapfs dir permissions
			header, err = tar.FileInfoHeader(currentPathStat, finalName)

			if err != nil {
				return err
			}

			header.Mode = 0777
			header.Typeflag = tar.TypeDir
		} else {
			if currentPathStat.IsDir() {
				return nil
			}
			header, err = tar.FileInfoHeader(currentPathStat, finalName)

			if err != nil {
				return err
			}
		}

		header.Name = filepath.ToSlash(finalName)

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if !currentPathStat.IsDir() {
			data, err := repositoryFiles.Open(currentPath)

			if err != nil {
				return err
			}

			if _, err := io.Copy(tw, data); err != nil {
				return err
			}

			data, err = repositoryFiles.Open(currentPath)

			if err != nil {
				return err
			}

			currentFileSha256 := sha256.New()

			if _, err = io.Copy(currentFileSha256, data); err != nil {
				return err
			}

			// stores file info to generate FILES.json
			galaxyFiles.Files = append(galaxyFiles.Files, types.GalaxyFile{
				Name:           header.Name,
				FType:          "file",
				ChecksumSha256: fmt.Sprintf("%x", currentFileSha256.Sum(nil)),
				Format:         1,
				ChecksumType:   "sha256",
			})
		}

		return nil
	})

	galaxyFilesSha256, err := addGalaxyFilesToTar(galaxyFiles, repositoryFiles, tw)

	if dependencies, err = addGalaxyManifestToTar(version, galaxyFiles, repositoryFiles, galaxyFilesSha256, tw); err != nil {
		return
	}

	// produce tar
	if err = tw.Close(); err != nil {
		return
	}

	// produce gzip
	if err = zr.Close(); err != nil {
		return
	}

	targetDir := filepath.Dir(targetPath)
	err = os.MkdirAll(targetDir, os.ModePerm)

	if err != nil {
		return
	}

	// Create the file
	out, err := os.Create(targetPath)
	if err != nil {
		return
	}

	defer out.Close()

	// Write the body to file
	if _, err = io.Copy(out, &tarGzBuffer); err != nil {
		return
	}

	fileTmp, err := os.Open(targetPath)
	defer fileTmp.Close()

	finalFileSha256 := sha256.New()

	if size, err = io.Copy(finalFileSha256, fileTmp); err != nil {
		return
	}

	result = types.DownloadedFileMetadata{
		Size:         size,
		Hash:         fmt.Sprintf("%x", finalFileSha256.Sum(nil)),
		Dependencies: dependencies,
	}
	return
}

func addGalaxyManifestToTar(version string, galaxyFiles types.GalaxyFiles, repositoryFiles fstest.MapFS, galaxyFilesSha256 string, tw *tar.Writer) (dependencies map[string]string, err error) {
	collectionInfo := &types.GalaxyCollectionInfo{}
	gyml, err := repositoryFiles.Glob("*/galaxy.yml")

	if err != nil || len(gyml) != 1 {
		err = fmt.Errorf("galaxy.yml not found")
		return
	}

	if err = yaml.Unmarshal(repositoryFiles[gyml[0]].Data, &collectionInfo); err != nil {
		return
	}

	if len(collectionInfo.Version) == 0 {
		collectionInfo.Version = version
	}

	dependencies = collectionInfo.Dependencies

	galaxyManifest := types.GalaxyManifest{
		CollectionInfo: *collectionInfo,
		FileManifestFile: types.GalaxyFile{
			Name:           "FILES.json",
			FType:          "file",
			ChecksumSha256: galaxyFilesSha256,
			Format:         1,
			ChecksumType:   "sha256",
		},
		Format: 1,
	}

	manifestJson, _ := json.Marshal(galaxyManifest)
	repositoryFiles["MANIFEST.json"] = &fstest.MapFile{
		Data: manifestJson,
		Mode: fs.FileMode(0555),
	}
	manifestStat, _ := repositoryFiles.Stat("MANIFEST.json")

	manifestHeader, err := tar.FileInfoHeader(manifestStat, "MANIFEST.json")

	if err != nil {
		return
	}

	if err = tw.WriteHeader(manifestHeader); err != nil {
		return
	}

	b := bytes.NewBuffer(manifestJson)
	if _, err = io.Copy(tw, b); err != nil {
		return
	}

	return
}

func addGalaxyFilesToTar(galaxyFiles types.GalaxyFiles, repositoryFiles fstest.MapFS, tw *tar.Writer) (fileSha256 string, err error) {
	filesJson, _ := json.Marshal(galaxyFiles)
	repositoryFiles["FILES.json"] = &fstest.MapFile{
		Data: filesJson,
		Mode: fs.FileMode(0555),
	}
	filesStat, _ := repositoryFiles.Stat("FILES.json")

	filesHeader, err := tar.FileInfoHeader(filesStat, "FILES.json")

	if err = tw.WriteHeader(filesHeader); err != nil {
		return
	}

	b := bytes.NewBuffer(filesJson)
	if _, err = io.Copy(tw, b); err != nil {
		return
	}

	filesHash := sha256.New()
	b = bytes.NewBuffer(filesJson)
	if _, err = io.Copy(filesHash, b); err != nil {
		return
	}

	fileSha256 = fmt.Sprintf("%x", filesHash.Sum(nil))
	return
}

func setupRouter() *gin.Engine {
	r := gin.Default()
	r.GET("/api/", func(c *gin.Context) {
		c.Writer.Header().Set("Content-Type", "application/json")
		c.JSON(http.StatusOK, gin.H{
			"description":     "GALAXY REST API",
			"current_version": "v2",
			"available_versions": gin.H{
				"v2": "v2/",
			},
			"server_version": "3.4.15",
			"version_name":   "Doin' it Right",
			"team_members":   []string{"chouseknecht", "cutwater", "alikins", "newswangerd", "awcrosby", "tima", "gregdek"},
		})
	})

	r.GET("/dl/:filename", func(c *gin.Context) {
		filename := c.Param("filename")
		filepath := fmt.Sprintf("%s/%s", TARGET_PATH, filename)

		fileTmp, err := os.Open(filepath)
		defer fileTmp.Close()

		fileName := path.Base(filepath)
		if len(filepath) == 0 || len(fileName) == 0 || err != nil {
			fmt.Println("=> " + filepath)
			c.Status(http.StatusFound)
			return
		}

		c.Header("Content-Type", "application/octet-stream")
		//Force browser download
		c.Header("Content-Disposition", "attachment; filename="+fileName)
		//Browser download or preview
		c.Header("Content-Disposition", "inline;filename="+fileName)
		c.Header("Content-Transfer-Encoding", "binary")
		c.Header("Cache-Control", "no-cache")

		c.File(filepath)
	})

	r.GET("/api/v2/collections/:namespace/:collection/", func(c *gin.Context) {
		namespace := c.Param("namespace")
		collection := c.Param("collection")

		tags, status, err := gitlab.GetTags(GITLAB_ENDPOINT, GITLAB_ROOT_GROUP, namespace, collection)

		if err != nil {
			c.Status(status)
			return
		}

		c.Writer.Header().Set("Content-Type", "application/json")

		c.JSON(http.StatusOK, gin.H{
			"id":   1,
			"href": fmt.Sprintf("https://galaxy.ansible.com/api/v2/collections/%s/%s/", namespace, collection),
			"name": collection,
			"namespace": gin.H{
				"id":   1,
				"href": "https://galaxy.ansible.com/api/v1/namespaces/1/",
				"name": namespace,
			},
			"versions_url": fmt.Sprintf("https://galaxy.ansible.com/api/v2/collections/%s/%s/versions/", namespace, collection),
			"latest_version": gin.H{
				"version":    tags[0].Name,
				"href":       fmt.Sprintf("https://galaxy.ansible.com/api/v2/collections/%s/%s/versions/%s/", namespace, collection, tags[0].Name),
				"deprecated": false,
				"created":    "2020-03-09T10:52:47.121204-04:00",
				"modified":   "2022-06-02T06:51:30.966601-04:00",
			},
		})
	})

	r.GET("/api/v2/collections/:namespace/:collection/versions/", func(c *gin.Context) {
		namespace := c.Param("namespace")
		collection := c.Param("collection")

		tags, status, err := gitlab.GetTags(GITLAB_ENDPOINT, GITLAB_ROOT_GROUP, namespace, collection)

		if err != nil {
			c.Status(status)
			return
		}

		result := types.Versions{}

		result.Count = len(tags)
		result.Results = []types.VersionLink{}

		for _, v := range tags {
			result.Results = append(result.Results, types.VersionLink{
				Version: v.Name,
				Href:    fmt.Sprintf("https://galaxy.ansible.com/api/v2/collections/%s/%s/versions/%s/", namespace, collection, v.Name),
			})
		}

		c.JSON(http.StatusOK, result)
	})

	r.GET("/api/v2/collections/:namespace/:collection/versions/:version", func(c *gin.Context) {
		namespace := c.Param("namespace")
		collection := c.Param("collection")
		version := c.Param("version")
		giturl := fmt.Sprintf("%s/%s/%s.%s/-/archive/%s/%s.%s-%s.tar.gz", GITLAB_ENDPOINT, GITLAB_ROOT_GROUP, namespace, collection, version, namespace, collection, version)
		localFile := fmt.Sprintf("%s/%s.%s-%s.tar.gz", TARGET_PATH, namespace, collection, version)
		localFileMetadata := fmt.Sprintf("%s/%s.%s-%s.tar.gz.metadata", TARGET_PATH, namespace, collection, version)

		if _, err := os.Stat(localFileMetadata); errors.Is(err, os.ErrNotExist) {
			downloadedMetadata, err := DownloadGitlabRepositoryAsGalaxyPackage(version, localFile, giturl)
			if err != nil {
				c.Status(http.StatusFailedDependency)
				return
			}

			downloadedMetadataBytes, _ := json.Marshal(downloadedMetadata)
			f, err := os.Create(localFile + ".metadata")

			if err != nil {
				return
			}

			defer f.Close()
			_, err = f.Write(downloadedMetadataBytes)
			if err != nil {
				return
			}
		}

		metadataJson, err := os.Open(localFileMetadata)

		if err != nil {
			return
		}

		byteValue, err := ioutil.ReadAll(metadataJson)

		if err != nil {
			return
		}

		downloadedMetadata := &types.DownloadedFileMetadata{}
		json.Unmarshal(byteValue, &downloadedMetadata)

		if err != nil {
			c.Status(http.StatusFailedDependency)
			panic(err)
		}

		if nil == downloadedMetadata.Dependencies {
			downloadedMetadata.Dependencies = map[string]string{}
		}

		downloadUrl := fmt.Sprintf("%s://%s:%s/dl/%s.%s-%s.tar.gz", SERVER_PROTOCOL, SERVER_BIND, SERVER_PORT, namespace, collection, version)
		c.JSON(http.StatusOK, gin.H{
			"id":           1,
			"href":         fmt.Sprintf("https://galaxy.ansible.com/api/v2/collections/%s/%s/versions/%s/", namespace, collection, version),
			"download_url": downloadUrl,
			"metadata": gin.H{
				"namespace":    namespace,
				"dependencies": downloadedMetadata.Dependencies,
			},
			"namespace": gin.H{
				"id":   1,
				"href": "https://galaxy.ansible.com/api/v1/namespaces/1/",
				"name": namespace,
			},
			"collection": gin.H{
				"id":   1,
				"href": fmt.Sprintf("https://galaxy.ansible.com/api/v2/collections/%s/%s/", namespace, collection),
				"name": collection,
			},
			"version": version,
			"artifact": gin.H{
				"filename": fmt.Sprintf("%s.%s-%s.tar.gz", namespace, collection, version),
				"size":     downloadedMetadata.Size,
				"sha256":   downloadedMetadata.Hash,
			},
		})
	})

	return r
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	r := setupRouter()

	fmt.Print(`
         _                 _ _     _   _
 ___ ___| |___ _ _ _ _ ___| |_|___| |_| |_
| . | .'| | .'|_'_| | |___| | | . |   |  _|
|_  |__,|_|__,|_,_|_  |   |_|_|_  |_|_|_|
|___|             |___|       |___|

`)
	fmt.Printf("Listening: %s:%s\n", SERVER_BIND, SERVER_PORT)

	r.Run(fmt.Sprintf("%s:%s", SERVER_BIND, SERVER_PORT))
}
