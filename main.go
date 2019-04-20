package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
	"time"
)

func main() {
	// fmt.Println(getV2Catalog("https://registry.suse.de/"))
	// fmt.Println(getV2Catalog("https://registry.suse.com/"))
	// fmt.Println(getV2Catalog("https://registry.opensuse.org"))
	// fmt.Println(getRepoTag("https://registry.suse.com/", "caasp/v4/caasp-dex"))
	// fmt.Println(getRepoTag("https://registry.opensuse.org/", "opensuse/tumbleweed"))

	// getAllReposWithTags("https://registry.suse.com/")
	// getAllReposWithTags("https://registry.suse.de/")
	getAllReposWithTags("https://registry.opensuse.org/")
	// getAllReposWithTags("https://r.j3ss.co/")
}

func getAllReposWithTags(baseURL string) {
	repoCatalog := getV2Catalog(baseURL)

	var wg sync.WaitGroup
	var lock sync.Mutex
	wg.Add(len(repoCatalog))
	reposWithTags := make(map[string][]string)

	for _, repo := range repoCatalog {
		time.Sleep(10 * time.Millisecond)
		go func(repo string) {
			defer wg.Done()
			tags := getRepoTag(baseURL, repo)
			lock.Lock()
			reposWithTags[repo] = tags
			lock.Unlock()
		}(repo)
	}
	wg.Wait()

	var repoSlice []string
	for k := range reposWithTags {
		repoSlice = append(repoSlice, k)
	}

	sort.Strings(repoSlice)

	fmt.Println()
	w := tabwriter.NewWriter(os.Stdout, 10, 0, 1, ' ', tabwriter.TabIndent|tabwriter.Debug)
	fmt.Fprintln(w, "\t REPO \t TAGS \t")
	fmt.Fprintln(w, "\t - - - - - - - - - - \t - - - - - - - - - - \t")
	for _, repo := range repoSlice {
		tags := strings.Join(reposWithTags[repo], "  ")
		if len(tags) > 45 {
			fmt.Fprintf(w, "\t %s\t %v\t\n", repo, tags[:45])
		} else {
			fmt.Fprintf(w, "\t %s\t %v\t\n", repo, tags)
		}
	}
	w.Flush()
}

func getRepoTag(baseURL, repo string) []string {
	parsedBaseURL, _ := url.Parse(baseURL)
	parsedBaseURL.Path = path.Join(parsedBaseURL.Path, "v2/", repo, "/tags/list")
	scope := "repository:" + repo + ":pull"
	token := getToken(parsedBaseURL.String(), scope)

	// Use token to get catalog
	req, _ := http.NewRequest("GET", parsedBaseURL.String(), nil)
	if strings.Compare(token, "-1") != 0 {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	// req.Header.Set("Authorization", "Bearer "+token)
	res, _ := http.DefaultClient.Do(req)
	bodyBytes, _ := ioutil.ReadAll(res.Body)
	var f map[string][]string
	_ = json.Unmarshal(bodyBytes, &f)

	return f["tags"]
}

func getV2Catalog(baseURL string) []string {
	parsedBaseURL, _ := url.Parse(baseURL)
	parsedBaseURL.Path = path.Join(parsedBaseURL.Path, "v2/_catalog")
	scope := "registry:catalog:*"
	token := getToken(parsedBaseURL.String(), scope)

	if len(token) > 2 {
		fmt.Println(token[:10])
	} else {
		fmt.Println("No token")
	}

	// Use token to get catalog
	req, _ := http.NewRequest("GET", parsedBaseURL.String(), nil)
	if strings.Compare(token, "-1") != 0 {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	// req.Header.Set("Authorization", "Bearer "+token)
	res, _ := http.DefaultClient.Do(req)
	bodyBytes, _ := ioutil.ReadAll(res.Body)

	var repositories []string
	var replacer = strings.NewReplacer("<", "", ">", "")

	// Status is OK
	if res.StatusCode >= 200 && res.StatusCode <= 299 {
		var f map[string][]string
		_ = json.Unmarshal(bodyBytes, &f)
		repositories = append(repositories, f["repositories"]...)
		headerLink := res.Header.Get("Link")
		for len(headerLink) != 0 {
			guardedLink := strings.Split(headerLink, ";")[0]
			cleanedLink := replacer.Replace(guardedLink)
			parsedBaseURL, _ := url.Parse(baseURL)
			parsedBaseURL.Path = path.Join(parsedBaseURL.Path, cleanedLink)
			unescapedURL, _ := url.QueryUnescape(parsedBaseURL.String())
			req2, _ := http.NewRequest("GET", unescapedURL, nil)
			req2.Header.Set("Authorization", "Bearer "+token)
			res, _ := http.DefaultClient.Do(req2)
			bodyBytes, _ := ioutil.ReadAll(res.Body)
			_ = json.Unmarshal(bodyBytes, &f)
			repositories = append(repositories, f["repositories"]...)
			headerLink = res.Header.Get("Link")
		}
	} else {
		fmt.Println("Failed to get repository list")
	}

	fmt.Println(len(repositories))
	return repositories
}

func getToken(baseURL, scope string) string {
	resp, err := http.Get(baseURL)
	if err != nil {
		log.Fatalln(err)
	}

	headerAuth := resp.Header.Get("www-authenticate")

	headerAuthLower := strings.ToLower(headerAuth)

	if strings.Count(headerAuthLower, "bearer") > 0 {
		fmt.Println("Using Bearer Auth")
	} else if strings.Count(headerAuthLower, "basic") > 0 {
		fmt.Println("Using Basic Auth")
		return "-1"
	} else {
		fmt.Println("Using No Auth")
		return "-1"
	}

	headerAuthSlice := strings.Split(headerAuth, ",")
	var authRealm, authService, authScope string
	for _, elem := range headerAuthSlice {
		if strings.Count(strings.ToLower(elem), "bearer") == 1 {
			elem = strings.Split(elem, " ")[1]
		}
		elem := strings.Replace(elem, "\"", "", -1)
		elemSplit := strings.Split(elem, "=")
		if len(elemSplit) != 2 {
			fmt.Printf("Incorrectly formatted Header Auth: %s\n", headerAuth)
		}
		authKey := elemSplit[0]
		authValue := elemSplit[1]
		switch authKey {
		case "realm":
			authRealm = authValue
		case "service":
			authService = authValue
		case "scope":
			authScope = authValue
		}
	}

	parsedRealm, err := url.Parse(authRealm)
	if err != nil {
		fmt.Println(err)
	}

	// Build query for token
	q := parsedRealm.Query()
	q.Set("service", authService)
	q.Set("scope", authScope)
	parsedRealm.RawQuery = q.Encode()

	// Make request for token
	resp2, err := http.Get(parsedRealm.String())
	if err != nil {
		log.Fatalln(err)
	}

	// Extract token
	body, _ := ioutil.ReadAll(resp2.Body)
	var f map[string]string
	_ = json.Unmarshal(body, &f)
	token := f["token"]

	return token
}
