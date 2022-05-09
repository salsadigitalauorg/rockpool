package rockpool

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
)

func (r *Rockpool) GiteaApiReq(method string, endpoint string, data []byte) (*http.Request, error) {
	url := fmt.Sprintf("http://gitea.%s/api/v1/%s", r.Hostname, endpoint)
	req, err := http.NewRequest(method, url, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json")
	return req, nil
}

func (r *Rockpool) GiteaApiCall(method string, endpoint string, token string, data []byte) (*http.Response, error) {
	req, err := r.GiteaApiReq(method, endpoint, data)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", "token "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (r *Rockpool) GiteaTokenApiCall(method string, data []byte, delete bool) (*http.Response, error) {
	endpoint := "users/rockpool/tokens"
	if delete {
		endpoint += "/" + string(data)
	}
	req, err := r.GiteaApiReq(method, endpoint, data)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("rockpool", "pass")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (r *Rockpool) GiteaHasToken() (string, error) {
	resp, err := r.GiteaTokenApiCall("GET", nil, false)
	if err != nil {
		return "", err
	}

	var res []struct {
		Id   json.Number `json:"id"`
		Name string      `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	for _, t := range res {
		if t.Name == "test" {
			return t.Id.String(), nil
		}
	}
	return "", nil
}

func (r *Rockpool) GiteaCreateToken() (string, error) {
	if id, err := r.GiteaHasToken(); err != nil {
		return "", err
	} else if id != "" {
		_, err := r.GiteaTokenApiCall("DELETE", []byte(id), true)
		if err != nil {
			return "", fmt.Errorf("error when deleting token: %s", err)
		}
	}

	data, _ := json.Marshal(map[string]string{"name": "test"})
	resp, err := r.GiteaTokenApiCall("POST", data, false)
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

func (r *Rockpool) GiteaHasTestRepo(token string) (bool, error) {
	resp, err := r.GiteaApiCall("GET", "user/repos", token, nil)
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

func (r *Rockpool) GiteaCreateRepo() {
	token, err := r.GiteaCreateToken()
	if err != nil {
		fmt.Println("[rockpool] error when fetching gitea token:", err)
		os.Exit(1)
	}

	if has, err := r.GiteaHasTestRepo(token); err != nil {
		fmt.Println("[rockpool] error when looking up gitea test repo:", err)
		os.Exit(1)
	} else if has {
		fmt.Println("[rockpool] gitea test repo already exists")
		return
	}

	fmt.Println("[rockpool] creating gitea test repo")
	data, _ := json.Marshal(map[string]string{"name": "test"})
	_, err = r.GiteaApiCall("POST", "user/repos", token, data)
	if err != nil {
		fmt.Println("[rockpool] unable to create gitea test repo:", err)
		os.Exit(1)
	}
}
