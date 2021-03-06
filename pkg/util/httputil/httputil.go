package httputil

import (
	"fmt"
	"time"

	"errors"

	"gopkg.in/resty.v0"
)

var (
	// ErrAlreadyExists 409返回该错误
	ErrAlreadyExists = errors.New("resource already exists")
	// ErrNotFound 404
	ErrNotFound = errors.New("resource not exists")
)

// Post create a resource
func Post(url string, body []byte) (string, error) {
	resp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Post(url)

	if err != nil {
		return "", fmt.Errorf("error: %v", err)
	}
	switch resp.StatusCode() {
	case 200, 201:
		return resp.String(), nil
	case 409:
		return "", ErrAlreadyExists
	default:
		return "", fmt.Errorf("post %s error: %v", url, resp.String())
	}
}

// Get get a resource
func Get(url string, timeout time.Duration) ([]byte, error) {
	resty.SetTimeout(timeout)
	resp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		Get(url)
	if err != nil {
		return nil, fmt.Errorf("error: %v", err)
	}
	if resp.StatusCode() == 404 {
		return nil, ErrNotFound
	}
	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("get resource %s error: %v", url, resp.String())
	}
	return resp.Body(), nil
}

// Delete a resource
func Delete(url string, timeout time.Duration) error {
	resty.SetTimeout(timeout)
	resp, err := resty.R().
		Delete(url)
	if err != nil {
		return fmt.Errorf("error: %v", err)
	}
	if resp.StatusCode() != 200 && resp.StatusCode() != 404 {
		return fmt.Errorf("delete service %s error: %v", url, resp.String())
	}
	return nil
}

// Patch a resource
func Patch(url string, body []byte, timeout time.Duration) error {
	resty.SetTimeout(timeout)
	resp, err := resty.R().
		SetHeader("Content-Type", "application/merge-patch+json").
		SetBody(body).
		Patch(url)
	if err != nil {
		return fmt.Errorf("error: %v", err)
	}
	if resp.StatusCode() != 200 && resp.StatusCode() != 404 {
		return fmt.Errorf("http: %v error: %v", url, resp.String())
	}
	return nil
}

// GetOk resource is ok
func GetOk(url string, timeout time.Duration) error {
	if _, err := Get(url, timeout); err != nil {
		return err
	}
	return nil
}
