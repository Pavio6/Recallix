package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"recallix/internal/shared"
)

func (s *Service) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, shared.APIResponse{
				Success: false,
				Error: &shared.APIError{
					Code:    shared.ErrUnauthorized.Code,
					Message: "Missing authorization header",
				},
			})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, shared.APIResponse{
				Success: false,
				Error: &shared.APIError{
					Code:    shared.ErrUnauthorized.Code,
					Message: "Invalid authorization format, expected 'Bearer <token>'",
				},
			})
			return
		}

		claims, err := s.ParseToken(parts[1])
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, shared.APIResponse{
				Success: false,
				Error: &shared.APIError{
					Code:    shared.ErrUnauthorized.Code,
					Message: err.Error(),
				},
			})
			return
		}

		c.Set("user_id", claims.UserID)
		c.Next()
	}
}
