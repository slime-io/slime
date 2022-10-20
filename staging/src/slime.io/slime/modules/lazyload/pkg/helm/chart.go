package helm

import (
	"io/fs"
	"path"
	"regexp"
	"strings"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/engine"
)

func LoadChartFromFS(fsys fs.FS, chartRoot string) (*chart.Chart, error) {
	files := []string{}
	fs.WalkDir(fsys, chartRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	})

	var bfs []*loader.BufferedFile
	for _, file := range files {
		bs, err := fs.ReadFile(fsys, file)
		if err != nil {
			return nil, err
		}
		bfs = append(bfs, &loader.BufferedFile{
			Name: strings.TrimPrefix(file, chartRoot+"/"),
			Data: bs,
		})
	}

	chrt, err := loader.LoadFiles(bfs)
	if err != nil {
		return nil, err
	}
	return chrt, nil
}

func RenderChartWithValues(chrt *chart.Chart, values map[string]interface{}) (map[string][]string, error) {
	data, err := chartutil.ToRenderValues(chrt, values, chartutil.ReleaseOptions{}, nil)
	if err != nil {
		return nil, err
	}

	manifests, err := engine.Render(chrt, data)
	if err != nil {
		return nil, err
	}
	splitedManifests := make(map[string][]string, len(manifests))
	for k, content := range manifests {
		if strings.HasSuffix(k, "NOTE.txt") {
			delete(manifests, k)
			continue
		}
		if strings.HasPrefix(path.Base(k), "_") {
			delete(manifests, k)
			continue
		}
		if strings.TrimSpace(content) == "" {
			delete(manifests, k)
			continue
		}
		splitedManifests[k] = splitManifests(content)
	}
	return splitedManifests, nil
}

var sep = regexp.MustCompile("(?:^|\\s*\n)---\\s*")

func splitManifests(bigFile string) []string {
	res := []string{}
	bigFileTmp := strings.TrimSpace(bigFile)
	docs := sep.Split(bigFileTmp, -1)
	var count int
	for _, d := range docs {
		if d == "" {
			continue
		}

		d = strings.TrimSpace(d)
		res = append(res, d)
		count = count + 1
	}
	return res
}

func RenderChart(chrt *chart.Chart) (map[string][]string, error) {
	return RenderChartWithValues(chrt, nil)
}
