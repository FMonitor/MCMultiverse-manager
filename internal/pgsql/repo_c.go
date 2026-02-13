package pgsql

import (
	"context"
	"database/sql"
	"encoding/json"
)

// c-layer contracts exposed to other packages.

type UserRepo interface {
	Create(ctx context.Context, user User) (int64, error)
	Read(ctx context.Context, id int64) (User, error)
	ReadByUUID(ctx context.Context, mcUUID string) (User, error)
	Update(ctx context.Context, user User) error
	Delete(ctx context.Context, id int64) error
}

type MapTemplateRepo interface {
	Create(ctx context.Context, template MapTemplate) (int64, error)
	Read(ctx context.Context, id int64) (MapTemplate, error)
	ReadByTag(ctx context.Context, tag string) (MapTemplate, error)
	ListByGameVersion(ctx context.Context, gameVersion string) ([]MapTemplate, error)
	ListGameVersions(ctx context.Context) ([]string, error)
	Update(ctx context.Context, template MapTemplate) error
	Delete(ctx context.Context, id int64) error
}

type ServerImageRepo interface {
	Create(ctx context.Context, image ServerImage) error
	Read(ctx context.Context, id string) (ServerImage, error)
	List(ctx context.Context) ([]ServerImage, error)
	Update(ctx context.Context, image ServerImage) error
	Delete(ctx context.Context, id string) error
}

type MapInstanceRepo interface {
	Create(ctx context.Context, inst MapInstance) (int64, error)
	Read(ctx context.Context, id int64) (MapInstance, error)
	Update(ctx context.Context, inst MapInstance) error
	Delete(ctx context.Context, id int64) error
}

type InstanceMemberRepo interface {
	Create(ctx context.Context, member InstanceMember) (int64, error)
	Read(ctx context.Context, id int64) (InstanceMember, error)
	Update(ctx context.Context, member InstanceMember) error
	Delete(ctx context.Context, id int64) error
}

type UserRequestRepo interface {
	Create(ctx context.Context, req UserRequest) (int64, error)
	Read(ctx context.Context, id int64) (UserRequest, error)
	ReadByRequestID(ctx context.Context, requestID string) (UserRequest, error)
	Update(ctx context.Context, req UserRequest) error
	Delete(ctx context.Context, id int64) error
	CreateAcceptedIfNotExists(ctx context.Context, requestID string, requestType string, actorUserID sql.NullInt64, targetInstanceID sql.NullInt64) (UserRequest, bool, error)
	MarkRequestResult(ctx context.Context, requestID string, status string, responsePayload json.RawMessage, errorCode sql.NullString, errorMsg sql.NullString) error
}

type Repos struct {
	User           UserRepo
	MapTemplate    MapTemplateRepo
	ServerImage    ServerImageRepo
	MapInstance    MapInstanceRepo
	InstanceMember InstanceMemberRepo
	UserRequest    UserRequestRepo
}

func NewRepos(connector SQLConnector) Repos {
	return Repos{
		User:           NewUserRepoI(connector),
		MapTemplate:    NewMapTemplateRepoI(connector),
		ServerImage:    NewServerImageRepoI(connector),
		MapInstance:    NewMapInstanceRepoI(connector),
		InstanceMember: NewInstanceMemberRepoI(connector),
		UserRequest:    NewUserRequestRepoI(connector),
	}
}
