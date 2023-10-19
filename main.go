package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
)

const store_path = `users.json`

type User struct {
	ID          string    `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	DisplayName string    `json:"display_name"`
	Email       string    `json:"email"`
}

type UserList map[string]User

type UserStore struct {
	Increment int      `json:"increment"`
	List      UserList `json:"list"`
}

func (store *UserStore) Inc() {
	store.Increment++
}

var (
	ErrUserNotFound = errors.New("user_not_found")
)

func main() {
	r := chi.NewRouter()

	r.Use(
		middleware.RequestID,
		middleware.RealIP,
		middleware.Logger,
		middleware.Recoverer,
		middleware.Timeout(60*time.Second),
	)

	r.Get("/", handleHome)
	r.Route("/api/v1/users", func(r chi.Router) {
		r.Get("/", HandleSearchUsers)
		r.Post("/", HandleCreateUser)
		r.Route("/{id}", func(r chi.Router) {
			r.Use(UserCtx)
			r.Get("/", HandleGetUser)
			r.Patch("/", HandleUpdateUser)
			r.Delete("/", HandleDeleteUser)
		})
	})
	err := http.ListenAndServe(":3333", r)
	if err != nil {
		log.Fatal("Error starting server: ", err)
	}
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(time.Now().String()))
}

func HandleGetUser(w http.ResponseWriter, r *http.Request) {
	user := GetUserCtx(r)
	render.JSON(w, r, user)
}

func ReadUserStore() (*UserStore, error) { //NOTE: New function
	data, err := os.ReadFile(store_path)
	if err != nil {
		return nil, err
	}

	var store UserStore
	err = json.Unmarshal(data, &store)
	if err != nil {
		return nil, err
	}

	return &store, nil
}

func SaveUserStore(store *UserStore) error { // NOTE: new function
	data, err := json.Marshal(store)
	if err != nil {
		return err
	}

	err = os.WriteFile(store_path, data, 0644)
	return err
}

func HandleSearchUsers(w http.ResponseWriter, r *http.Request) {
	store, err := ReadUserStore()
	if err != nil {
		render.Render(w, r, ErrInvalidRequest(err))
		return
	}
	render.JSON(w, r, store.List)
}

func HandleCreateUser(w http.ResponseWriter, r *http.Request) {
	store, err := ReadUserStore()
	if err != nil {
		render.Render(w, r, ErrInvalidRequest(err))
		return
	}

	var request CreateUserRequest
	if err := render.Bind(r, &request); err != nil {
		render.Render(w, r, ErrInvalidRequest(err))
		return
	}

	store.Inc()
	id := strconv.Itoa(store.Increment)
	user := User{
		ID:          id,
		CreatedAt:   time.Now(),
		DisplayName: request.DisplayName,
		Email:       request.Email,
	}

	store.List[id] = user
	if err := SaveUserStore(store); err != nil {
		render.Render(w, r, ErrInvalidRequest(err))
		return
	}

	render.Status(r, http.StatusCreated)
	render.JSON(w, r, map[string]interface{}{
		"user_id": id,
	})
}

func HandleUpdateUser(w http.ResponseWriter, r *http.Request) {
	store, err := ReadUserStore()
	if err != nil {
		render.Render(w, r, ErrInvalidRequest(err))
		return
	}

	user := GetUserCtx(r)
	var request UpdateUserRequest
	if err := render.Bind(r, &request); err != nil {
		render.Render(w, r, ErrInvalidRequest(err))
		return
	}

	user.DisplayName = request.DisplayName
	store.List[user.ID] = user

	if err := SaveUserStore(store); err != nil {
		render.Render(w, r, ErrInvalidRequest(err))
		return
	}
	render.Status(r, http.StatusNoContent)
}

func HandleDeleteUser(w http.ResponseWriter, r *http.Request) {
	store, err := ReadUserStore()
	if err != nil {
		render.Render(w, r, ErrInvalidRequest(err))
		return
	}
	user := GetUserCtx(r)
	delete(store.List, user.ID)

	if err := SaveUserStore(store); err != nil {
		render.Render(w, r, ErrInvalidRequest(err))
		return
	}
	render.Status(r, http.StatusNoContent)
}

func GetUserCtx(r *http.Request) User {
	return r.Context().Value("user").(User)
}

func UserCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "id")
		store, err := ReadUserStore()
		if err != nil {
			render.Render(w, r, ErrInvalidRequest(err))
			return
		}

		user, found := store.List[userID]
		if !found {
			render.Render(w, r, ErrInvalidRequest(ErrUserNotFound))
			return
		}
		ctx := context.WithValue(r.Context(), "user", user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type CreateUserRequest struct {
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

func (c *CreateUserRequest) Bind(r *http.Request) error { return nil }
func (u *UpdateUserRequest) Bind(r *http.Request) error { return nil }

type UpdateUserRequest struct {
	DisplayName string `json:"display_name"`
}

type ErrResponse struct {
	Err            error `json:"-"`
	HTTPStatusCode int   `json:"-"`

	StatusText string `json:"status"`
	AppCode    int64  `json:"code,omitempty"`
	ErrorText  string `json:"error,omitempty"`
}

func (e *ErrResponse) Render(w http.ResponseWriter, r *http.Request) error {
	render.Status(r, e.HTTPStatusCode)
	return nil
}

func ErrInvalidRequest(err error) render.Renderer {
	return &ErrResponse{
		Err:            err,
		HTTPStatusCode: 400,
		StatusText:     "Invalid request.",
		ErrorText:      err.Error(),
	}
}
