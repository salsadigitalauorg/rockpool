package gitea

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/salsadigitalauorg/rockpool/pkg/interceptor"
	"github.com/salsadigitalauorg/rockpool/pkg/platform"

	log "github.com/sirupsen/logrus"
)

func ApiReq(method string, endpoint string, data []byte) (*http.Request, error) {
	url := fmt.Sprintf("http://gitea.lagoon.%s/api/v1/%s", platform.Hostname(), endpoint)
	req, err := http.NewRequest(method, url, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json")
	return req, nil
}

func ApiCall(method string, endpoint string, token string, data []byte) (*http.Response, error) {
	req, err := ApiReq(method, endpoint, data)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", "token "+token)

	client := &http.Client{
		Transport: interceptor.New(),
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func TokenApiCall(method string, data []byte, delete bool) (*http.Response, error) {
	endpoint := "users/rockpool/tokens"
	if delete {
		endpoint += "/" + string(data)
	}
	req, err := ApiReq(method, endpoint, data)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("rockpool", "pass")

	client := &http.Client{
		Transport: interceptor.New(),
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func HasToken() (string, error) {
	var tokens []struct {
		Id   json.Number `json:"id"`
		Name string      `json:"name"`
	}

	// Retry the token API call for 1 minute to allow for resource creation to
	// finish.
	done := false
	retries := 12
	var resp *http.Response
	var err error
	var dump []byte
	for !done && retries > 0 {
		resp, err = TokenApiCall("GET", nil, false)
		if err != nil {
			return "", fmt.Errorf("error calling gitea token endpoint: %s", err)
		}
		dump, _ = httputil.DumpResponse(resp, true)

		if err = json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
			if err.Error() == "invalid character '<' looking for beginning of value" {
				time.Sleep(5 * time.Second)
				retries--
				continue
			}
			return "", fmt.Errorf("error decoding gitea token: %s. response: %s", err, string(dump))
		}
		done = true
	}
	if !done {
		return "", fmt.Errorf("error decoding gitea token: %s. response: %s", err, string(dump))
	}
	for _, t := range tokens {
		if t.Name == "test" {
			return t.Id.String(), nil
		}
	}
	return "", nil
}

func CreateToken() (string, error) {
	if id, err := HasToken(); err != nil {
		return "", fmt.Errorf("error checking gitea token: %s", err)
	} else if id != "" {
		_, err := TokenApiCall("DELETE", []byte(id), true)
		if err != nil {
			return "", fmt.Errorf("error when deleting token: %s", err)
		}
	}

	data, _ := json.Marshal(map[string]string{"name": "test"})
	resp, err := TokenApiCall("POST", data, false)
	if err != nil {
		return "", err
	}

	var res struct {
		Token   string `json:"sha1"`
		Message string `json:"message"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}

	if res.Message != "" {
		return "", errors.New(res.Message)
	}
	return res.Token, nil
}

func HasTestRepo(token string) (bool, error) {
	resp, err := ApiCall("GET", "user/repos", token, nil)
	if err != nil {
		return false, err
	}
	var res []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return false, err
	}
	for _, t := range res {
		if t.Name == "test" {
			return true, nil
		}
	}
	return false, nil
}

func CreateRepo() {
	token, err := CreateToken()
	if err != nil {
		log.WithField("err", err).Fatal("error creating gitea token")
	}

	if has, err := HasTestRepo(token); err != nil {
		log.WithField("err", err).Fatal("error looking up gitea test repo")
	} else if has {
		log.Debug("gitea test repo already exists")
		return
	}

	log.Info("creating gitea test repo")
	data, _ := json.Marshal(map[string]string{"name": "test"})
	_, err = ApiCall("POST", "user/repos", token, data)
	if err != nil {
		log.WithField("err", err).Fatal("unable to create gitea test repo")
	}
}
