package leetcode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)


type graphqlRequest struct {
	Query string `json:"query"`
	Variables map[string]any `json:"variables"`
}

type graphqlResponse struct {
	Data json.RawMessage `json:"data"`
}

func (w *Worker) configureCsrfToken() error {
	var response, err = w.httpClient.Get("https://leetcode.com/graphql/")
	if err != nil {
		slog.Error("Failed to send HTTP request")
		return err
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusBadRequest {
		slog.Warn("HTTP request failed", "status", response.Status)
	}

	for _, cookie := range response.Cookies() {
		if cookie.Name == "csrftoken" {
			w.csrfToken = cookie.Value
			return nil
		}
	}

	slog.Error("Missing CSRF token cookie in response from site root")
	return fmt.Errorf("Missing CSRF token cookie in response from site root")
}

func (w *Worker) sendGraphqlQuery(query string, variables map[string]any) (*json.RawMessage, error) {
	var requestData = graphqlRequest{
		Query: strings.ReplaceAll(query, "\n", " "),
		Variables: variables,
	}
	var requestBody, marshalErr = json.Marshal(requestData)
	if marshalErr != nil {
		slog.Error("Failed to marshal GraphQL query into JSON body of HTTP request")
		return nil, marshalErr
	}

	var request, requestErr = http.NewRequest("POST", "https://leetcode.com/graphql/", bytes.NewReader(requestBody))
	if requestErr != nil {
		slog.Error("Failed to create HTTP request")
		return nil, requestErr
	}

	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("Referer", "https://leetcode.com")
	request.Header.Add("x-csrftoken", w.csrfToken)
	var csrfCookie = http.Cookie{
		Name: "csrftoken",
		Value: w.csrfToken,
	}
	request.AddCookie(&csrfCookie)

	var client = http.Client{}
	var response, responseErr = client.Do(request)
	if responseErr != nil {
		slog.Error("Failed to send HTTP request")
		return nil, responseErr
	}
	defer response.Body.Close()

	var responseBody, readErr = io.ReadAll(response.Body)
	if readErr != nil {
		slog.Error("Failed to read response body")
		return nil, readErr
	}

	if response.StatusCode != http.StatusOK {
		slog.Error("HTTP request failed", "status", response.Status, "body", string(responseBody))
		return nil, fmt.Errorf("HTTP request returned %s with body: %s", response.Status, string(responseBody))
	}

	var responseData graphqlResponse
	if unmarshalErr := json.Unmarshal(responseBody, &responseData); unmarshalErr != nil {
		slog.Error("Failed to unmarshal JSON response", "body", string(responseBody))
		return nil, unmarshalErr
	}

	return &responseData.Data, nil
}
