package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/tylerhardison/race-condition-kingdom/internal/auth"
	"github.com/tylerhardison/race-condition-kingdom/internal/database"
	"github.com/tylerhardison/race-condition-kingdom/pkg/config"
)

// AuthHandler handles HTTP authentication requests
type AuthHandler struct {
	authService *auth.AuthService
	logger      *logrus.Logger
	templates   *template.Template
}

// AuthRequest represents an authentication request
type AuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// AuthResponse represents an authentication response
type AuthResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Token   string `json:"token,omitempty"`
}

type registerPageData struct {
	Token         string
	Error         string
	Username      string
	Email         string
	CharacterName string
}

func main() {
	// Initialize logger
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	logger.SetFormatter(&logrus.JSONFormatter{})

	// Load configuration
	cfg := config.LoadFromEnv()

	// Initialize database
	db, err := database.New(cfg, logger)
	if err != nil {
		logger.WithError(err).Fatal("Failed to initialize database")
	}
	defer db.Close()

	// Initialize auth service
	authService := auth.NewAuthService(db.GetPostgreSQLDB(), db.GetRedisClient(), cfg, logger)

	// Initialize HTTP handler
	handler := &AuthHandler{
		authService: authService,
		logger:      logger,
	}

	// Load templates
	if err := handler.loadTemplates(); err != nil {
		logger.WithError(err).Fatal("Failed to load templates")
	}

	// Setup routes
	router := mux.NewRouter()

	// Authentication routes
	router.HandleFunc("/auth", handler.handleAuthPage).Methods("GET")
	router.HandleFunc("/auth", handler.handleAuthSubmit).Methods("POST")
	router.HandleFunc("/register", handler.handleRegisterPage).Methods("GET")
	router.HandleFunc("/register", handler.handleRegisterSubmit).Methods("POST")
	router.HandleFunc("/api/auth", handler.handleAPIAuth).Methods("POST")
	router.HandleFunc("/health", handler.handleHealth).Methods("GET")

	// Static files (if needed)
	router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))))

	// Create HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.HTTPPort),
		Handler:      router,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
	}

	// Start server in goroutine
	go func() {
		logger.WithField("address", server.Addr).Info("Starting auth service HTTP server")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.WithError(err).Fatal("HTTP server failed")
		}
	}()

	// Wait for interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	logger.Info("Shutting down auth service...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.WithError(err).Error("Server shutdown error")
	}

	logger.Info("Auth service stopped")
}

func (h *AuthHandler) handleRegisterPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Missing authentication token", http.StatusBadRequest)
		return
	}
	if _, err := h.authService.ValidateAuthToken(token); err != nil {
		h.logger.WithError(err).Warn("Invalid auth token for registration")
		http.Error(w, "Invalid or expired authentication token", http.StatusBadRequest)
		return
	}
	h.renderRegisterPage(w, registerPageData{Token: token})
}

func (h *AuthHandler) handleRegisterSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}
	data := registerPageData{
		Token:         r.FormValue("token"),
		Username:      r.FormValue("username"),
		Email:         r.FormValue("email"),
		CharacterName: r.FormValue("character_name"),
	}
	if data.Token == "" || r.FormValue("password") == "" {
		data.Error = "All fields are required."
		h.renderRegisterPage(w, data)
		return
	}

	sessionID, err := h.authService.ValidateAuthToken(data.Token)
	if err != nil {
		h.logger.WithError(err).Warn("Invalid auth token during registration")
		http.Error(w, "Invalid or expired authentication token", http.StatusBadRequest)
		return
	}

	user, character, err := h.authService.RegisterPlayer(data.Username, data.Email, r.FormValue("password"), data.CharacterName)
	if err != nil {
		data.Error = "Could not create that account. " + err.Error()
		h.renderRegisterPage(w, data)
		return
	}
	if err := h.authService.LinkTokenToSession(data.Token, sessionID, user.ID, character.ID); err != nil {
		h.logger.WithError(err).Error("Failed to link registered user to session")
		http.Error(w, "Authentication linking failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	if err := h.templates.ExecuteTemplate(w, "success.html", struct {
		Username      string
		CharacterName string
	}{Username: user.Username, CharacterName: character.Name}); err != nil {
		h.logger.WithError(err).Error("Failed to render registration success")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h *AuthHandler) renderRegisterPage(w http.ResponseWriter, data registerPageData) {
	w.Header().Set("Content-Type", "text/html")
	if err := h.templates.ExecuteTemplate(w, "register.html", data); err != nil {
		h.logger.WithError(err).Error("Failed to render registration page")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// handleAuthPage serves the authentication page
func (h *AuthHandler) handleAuthPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Missing authentication token", http.StatusBadRequest)
		return
	}

	// Validate token exists and get session info
	sessionID, err := h.authService.ValidateAuthToken(token)
	if err != nil {
		h.logger.WithError(err).Warn("Invalid auth token")
		http.Error(w, "Invalid or expired authentication token", http.StatusBadRequest)
		return
	}

	// Render authentication form
	data := struct {
		Token     string
		SessionID string
	}{
		Token:     token,
		SessionID: sessionID,
	}

	w.Header().Set("Content-Type", "text/html")
	if err := h.templates.ExecuteTemplate(w, "auth.html", data); err != nil {
		h.logger.WithError(err).Error("Failed to render auth template")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// handleAuthSubmit handles authentication form submission
func (h *AuthHandler) handleAuthSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	token := r.FormValue("token")
	username := r.FormValue("username")
	password := r.FormValue("password")

	if token == "" || username == "" || password == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	// Validate token and get session
	sessionID, err := h.authService.ValidateAuthToken(token)
	if err != nil {
		h.logger.WithError(err).Warn("Invalid auth token during submission")
		http.Error(w, "Invalid or expired authentication token", http.StatusBadRequest)
		return
	}

	// Authenticate user
	user, err := h.authService.AuthenticateUser(username, password)
	if err != nil {
		h.logger.WithError(err).Info("Authentication failed")

		// Render error page
		data := struct {
			Token    string
			Error    string
			Username string
		}{
			Token:    token,
			Error:    "Invalid username or password",
			Username: username,
		}

		w.Header().Set("Content-Type", "text/html")
		if err := h.templates.ExecuteTemplate(w, "auth.html", data); err != nil {
			h.logger.WithError(err).Error("Failed to render auth template with error")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	// Get user's characters
	characters, err := h.authService.GetUserCharacters(user.ID)
	if err != nil {
		h.logger.WithError(err).Error("Failed to get user characters")
		http.Error(w, "Failed to load character data", http.StatusInternalServerError)
		return
	}

	if len(characters) == 0 {
		http.Error(w, "No characters found. Character creation not yet implemented.", http.StatusBadRequest)
		return
	}

	// For now, use the first character
	character := characters[0]

	// Link token to session
	if err := h.authService.LinkTokenToSession(token, sessionID, user.ID, character.ID); err != nil {
		h.logger.WithError(err).Error("Failed to link token to session")
		http.Error(w, "Authentication linking failed", http.StatusInternalServerError)
		return
	}

	// Render success page
	data := struct {
		Username      string
		CharacterName string
	}{
		Username:      user.Username,
		CharacterName: character.Name,
	}

	w.Header().Set("Content-Type", "text/html")
	if err := h.templates.ExecuteTemplate(w, "success.html", data); err != nil {
		h.logger.WithError(err).Error("Failed to render success template")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	h.logger.WithFields(logrus.Fields{
		"user_id":      user.ID,
		"username":     user.Username,
		"character_id": character.ID,
		"session_id":   sessionID,
	}).Info("Authentication successful")
}

// handleAPIAuth handles API-based authentication
func (h *AuthHandler) handleAPIAuth(w http.ResponseWriter, r *http.Request) {
	var req AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	token := r.Header.Get("X-Auth-Token")
	if token == "" {
		http.Error(w, "Missing auth token", http.StatusBadRequest)
		return
	}

	// Validate token and get session
	sessionID, err := h.authService.ValidateAuthToken(token)
	if err != nil {
		response := AuthResponse{
			Success: false,
			Message: "Invalid or expired token",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Authenticate user
	user, err := h.authService.AuthenticateUser(req.Username, req.Password)
	if err != nil {
		response := AuthResponse{
			Success: false,
			Message: "Invalid username or password",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Get user's characters
	characters, err := h.authService.GetUserCharacters(user.ID)
	if err != nil {
		response := AuthResponse{
			Success: false,
			Message: "Failed to load character data",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	if len(characters) == 0 {
		response := AuthResponse{
			Success: false,
			Message: "No characters found",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Use first character
	character := characters[0]

	// Link token to session
	if err := h.authService.LinkTokenToSession(token, sessionID, user.ID, character.ID); err != nil {
		response := AuthResponse{
			Success: false,
			Message: "Authentication linking failed",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Success response
	response := AuthResponse{
		Success: true,
		Message: "Authentication successful",
		Token:   token,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	h.logger.WithFields(logrus.Fields{
		"user_id":      user.ID,
		"username":     user.Username,
		"character_id": character.ID,
		"session_id":   sessionID,
	}).Info("API authentication successful")
}

// handleHealth handles health check requests
func (h *AuthHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":    "healthy",
		"service":   "auth-service",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// loadTemplates loads HTML templates
func (h *AuthHandler) loadTemplates() error {
	// Keep the templates in the binary so the service can start from any
	// read-only working directory.
	authTemplate := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Race Condition Kingdom - Authentication</title>
    <style>
        body { font-family: 'Courier New', monospace; background: #1a1a1a; color: #00ff00; margin: 0; padding: 20px; }
        .container { max-width: 500px; margin: 0 auto; background: #000; padding: 30px; border: 2px solid #00ff00; border-radius: 10px; }
        .title { text-align: center; color: #ffff00; margin-bottom: 30px; font-size: 24px; }
        .form-group { margin-bottom: 20px; }
        label { display: block; margin-bottom: 5px; color: #00ffff; }
        input[type="text"], input[type="password"] { width: 100%; padding: 10px; background: #333; border: 1px solid #666; color: #fff; font-family: inherit; }
        input[type="submit"] { background: #00ff00; color: #000; padding: 12px 30px; border: none; cursor: pointer; font-family: inherit; font-weight: bold; }
        input[type="submit"]:hover { background: #00cc00; }
        .error { color: #ff0000; margin-bottom: 20px; padding: 10px; border: 1px solid #ff0000; background: #330000; }
        .info { color: #ffff00; margin-bottom: 20px; font-size: 14px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="title">🏰 RACE CONDITION KINGDOM 🏰</div>
        <div class="info">Please enter your credentials to authenticate your telnet session.</div>
        
        {{if .Error}}
        <div class="error">{{.Error}}</div>
        {{end}}
        
        <form method="POST" action="/auth">
            <input type="hidden" name="token" value="{{.Token}}">
            
            <div class="form-group">
                <label for="username">Username:</label>
                <input type="text" id="username" name="username" value="{{.Username}}" required autofocus>
            </div>
            
            <div class="form-group">
                <label for="password">Password:</label>
                <input type="password" id="password" name="password" required>
            </div>
            
            <div class="form-group">
                <input type="submit" value="AUTHENTICATE">
            </div>
        </form>
        
        <div class="info">
            New here? <a href="/register?token={{.Token}}">Create an account and character.</a><br>
            Local development account: admin / admin123
        </div>
    </div>
</body>
</html>`

	registerTemplate := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Join Race Condition Kingdom</title>
    <style>
        body { font-family: 'Courier New', monospace; background: #1a1a1a; color: #00ff00; margin: 0; padding: 20px; }
        .container { max-width: 500px; margin: 0 auto; background: #000; padding: 30px; border: 2px solid #00ff00; border-radius: 10px; }
        .title { text-align: center; color: #ffff00; margin-bottom: 20px; font-size: 24px; }
        .form-group { margin-bottom: 16px; } label { display: block; margin-bottom: 5px; color: #00ffff; }
        input { box-sizing: border-box; width: 100%; padding: 10px; background: #333; border: 1px solid #666; color: #fff; font-family: inherit; }
        input[type="submit"] { width: auto; background: #00ff00; color: #000; border: none; cursor: pointer; font-weight: bold; }
        .error { color: #ff8080; margin-bottom: 16px; padding: 10px; border: 1px solid #ff0000; background: #330000; }
        .info { color: #ffff00; font-size: 14px; }
        a { color: #00ffff; }
    </style>
</head>
<body>
    <div class="container">
        <div class="title">JOIN THE KINGDOM</div>
        <div class="info">Create one account and the character who will enter Town Square.</div>
        {{if .Error}}<div class="error">{{.Error}}</div>{{end}}
        <form method="POST" action="/register">
            <input type="hidden" name="token" value="{{.Token}}">
            <div class="form-group"><label>Account username</label><input name="username" value="{{.Username}}" required autofocus></div>
            <div class="form-group"><label>Email</label><input type="email" name="email" value="{{.Email}}" required></div>
            <div class="form-group"><label>Password (8+ characters)</label><input type="password" name="password" required></div>
            <div class="form-group"><label>Character name</label><input name="character_name" value="{{.CharacterName}}" required></div>
            <input type="submit" value="CREATE CHARACTER">
        </form>
        <p class="info"><a href="/auth?token={{.Token}}">I already have an account.</a></p>
    </div>
</body>
</html>`

	successTemplate := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Authentication Successful</title>
    <style>
        body { font-family: 'Courier New', monospace; background: #1a1a1a; color: #00ff00; margin: 0; padding: 20px; }
        .container { max-width: 500px; margin: 0 auto; background: #000; padding: 30px; border: 2px solid #00ff00; border-radius: 10px; text-align: center; }
        .title { color: #ffff00; margin-bottom: 30px; font-size: 24px; }
        .success { color: #00ff00; font-size: 18px; margin-bottom: 20px; }
        .info { color: #00ffff; margin-bottom: 20px; }
        .character { color: #ffff00; font-weight: bold; }
    </style>
</head>
<body>
    <div class="container">
        <div class="title">🎉 AUTHENTICATION SUCCESSFUL! 🎉</div>
        <div class="success">Welcome back, {{.Username}}!</div>
        <div class="info">You are now logged in as character: <span class="character">{{.CharacterName}}</span></div>
        <div class="info">You can now return to your telnet client and type 'check' to continue.</div>
        <div class="info">This window can be safely closed.</div>
    </div>
</body>
</html>`

	templates, err := template.New("auth.html").Parse(authTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse templates: %w", err)
	}
	if _, err := templates.New("success.html").Parse(successTemplate); err != nil {
		return fmt.Errorf("failed to parse success template: %w", err)
	}
	if _, err := templates.New("register.html").Parse(registerTemplate); err != nil {
		return fmt.Errorf("failed to parse registration template: %w", err)
	}

	h.templates = templates
	return nil
}
