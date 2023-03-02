package servegit

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/sourcegraph/log/logtest"
)

const testAddress = "test.local:3939"

func TestReposHandler(t *testing.T) {
	cases := []struct {
		name  string
		repos [][]string
	}{
		{
			name: "empty",
		}, {
			name:  "simple",
			repos: [][]string{{"project1", "project2"}},
		}, {
			name: "nested",
			repos: [][]string{
				{"project1", "project2", "dir/project3", "dir/project4.bare"},
			},
		}, {
			name: "nested and multiple roots",
			repos: [][]string{
				{"project1", "project2", "dir/project3", "dir/project4.bare"},
				{"project5", "dir/project6"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var (
				want  []Repo
				roots []string
			)
			for i := 0; i < len(tc.repos); i++ {
				currentRoot := gitInitRepos(t, tc.repos[i]...)
				roots = append(roots, currentRoot)

				// os may store temp cache directories under /private
				parent := "/private"
				_, err := os.ReadDir(parent + currentRoot)
				if err == nil {
					currentRoot = filepath.Join(parent, filepath.FromSlash(currentRoot))
				}

				for _, name := range tc.repos[i] {
					isBare := strings.HasSuffix(name, ".bare")
					uri := path.Join("/repos", currentRoot, name)
					clonePath := uri
					if !isBare {
						clonePath += "/.git"
					}
					want = append(want, Repo{Name: name, URI: uri, ClonePath: clonePath})

				}
			}

			h := (&Serve{
				Logger: logtest.Scoped(t),
				Addr:   testAddress,
			}).handler()

			testReposHandler(t, h, want, roots)
		})
	}
}

func testReposHandler(t *testing.T, h http.Handler, repos []Repo, roots []string) {
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	get := func(path string) string {
		res, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	post := func(path string, body []byte) string {
		res, err := http.Post(ts.URL+path, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	// Check we have some known strings on the index page
	index := get("/")
	for _, sub := range []string{"http://" + testAddress, "/v1/list-repos", "/repos/"} {
		if !strings.Contains(index, sub) {
			t.Errorf("index page does not contain substring %q", sub)
		}
	}
	for _, rootDir := range roots {
		// repos page will list the top-level dirs
		list := get(filepath.Join("/repos/", rootDir))
		for _, repo := range repos {
			if path.Dir(repo.URI) != "/repos" {
				continue
			}
			if !strings.Contains(repo.Name, "/") && !strings.Contains(list, repo.Name) {
				t.Errorf("repos page does not contain substring %q", repo.Name)
			}
		}
	}

	// check our API response
	type Response struct{ Items []Repo }
	var want, got Response
	want.Items = repos
	reqBody, err := json.Marshal(ListReposRequest{Roots: roots})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(post("/v1/list-repos", reqBody)), &got); err != nil {
		t.Fatal(err)
	}
	opts := []cmp.Option{
		cmpopts.EquateEmpty(),
		cmpopts.SortSlices(func(a, b Repo) bool { return a.Name < b.Name }),
	}
	if !cmp.Equal(want, got, opts...) {
		t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(want, got, opts...))
	}
}

func gitInitBare(t *testing.T, path string) {
	if err := exec.Command("git", "init", "--bare", path).Run(); err != nil {
		t.Fatal(err)
	}
}

func gitInit(t *testing.T, path string) {
	cmd := exec.Command("git", "init")
	cmd.Dir = path
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
}

func gitInitRepos(t *testing.T, names ...string) string {
	root := t.TempDir()
	root = filepath.Join(root, "repos-root")

	for _, name := range names {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Fatal(err)
		}

		if strings.HasSuffix(p, ".bare") {
			gitInitBare(t, p)
		} else {
			gitInit(t, p)
		}
	}

	return root
}

func TestIgnoreGitSubmodules(t *testing.T) {
	root := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root, "dir"), os.ModePerm); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "dir", ".git"), []byte("ignore me please"), os.ModePerm); err != nil {
		t.Fatal(err)
	}

	repos, err := (&Serve{
		Logger: logtest.Scoped(t),
	}).ReposUnion([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 0 {
		t.Fatalf("expected no repos, got %v", repos)
	}
}

func TestIsBareRepo(t *testing.T) {
	dir := t.TempDir()

	gitInitBare(t, dir)

	if !isBareRepo(dir) {
		t.Errorf("Path %s it not a bare repository", dir)
	}
}

func TestEmptyDirIsNotBareRepo(t *testing.T) {
	dir := t.TempDir()

	if isBareRepo(dir) {
		t.Errorf("Path %s it falsey detected as a bare repository", dir)
	}
}
