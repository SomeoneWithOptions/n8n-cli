package n8n

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// TagsPath is the collection endpoint for workflow tags.
const TagsPath = "/tags"

// Tag is one workflow tag. Name is the only writable field: the API marks id,
// createdAt and updatedAt read-only and rejects any other property.
type Tag struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// tagRequest is the write document for create and update. It is separate from
// [Tag] so read-only fields are never sent back to the server.
type tagRequest struct {
	Name string `json:"name"`
}

// ListTags returns one cursor-paginated page of tags.
func (c *Client) ListTags(ctx context.Context, opts ListOptions) (Page[Tag], error) {
	if err := opts.Validate(); err != nil {
		return Page[Tag]{}, err
	}
	var page Page[Tag]
	if _, err := c.Do(ctx, Request{Path: TagsPath, Query: opts.Apply(nil)}, &page); err != nil {
		return Page[Tag]{}, err
	}
	return page, nil
}

// CreateTag creates a tag. Tag names are unique per instance, so a duplicate
// name answers 409; see [IsConflict].
func (c *Client) CreateTag(ctx context.Context, name string) (*Tag, error) {
	if err := validateTagName(name); err != nil {
		return nil, err
	}
	var created Tag
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   TagsPath,
		Body:   tagRequest{Name: name},
	}, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// GetTag returns one tag by ID. id is escaped as a single path segment.
func (c *Client) GetTag(ctx context.Context, id string) (*Tag, error) {
	if err := validateTagID(id); err != nil {
		return nil, err
	}
	var tag Tag
	if _, err := c.Do(ctx, Request{Path: PathJoin("tags", id)}, &tag); err != nil {
		return nil, err
	}
	return &tag, nil
}

// UpdateTag replaces a tag. The request is a full replacement, but name is the
// only writable field, so renaming is the whole operation. A name already in
// use answers 409.
func (c *Client) UpdateTag(ctx context.Context, id, name string) (*Tag, error) {
	if err := validateTagID(id); err != nil {
		return nil, err
	}
	if err := validateTagName(name); err != nil {
		return nil, err
	}
	var updated Tag
	if _, err := c.Do(ctx, Request{
		Method: http.MethodPut,
		Path:   PathJoin("tags", id),
		Body:   tagRequest{Name: name},
	}, &updated); err != nil {
		return nil, err
	}
	return &updated, nil
}

// DeleteTag removes a tag from the instance and from every workflow carrying
// it, and returns the deleted tag as the server last saw it.
func (c *Client) DeleteTag(ctx context.Context, id string) (*Tag, error) {
	if err := validateTagID(id); err != nil {
		return nil, err
	}
	var deleted Tag
	if _, err := c.Do(ctx, Request{Method: http.MethodDelete, Path: PathJoin("tags", id)}, &deleted); err != nil {
		return nil, err
	}
	return &deleted, nil
}

func validateTagID(id string) error { return validateTagText("ID", id) }

func validateTagName(name string) error { return validateTagText("name", name) }

func validateTagText(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("tag %s is required", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("tag %s must not start or end with whitespace", field)
	}
	return nil
}
