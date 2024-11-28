package ginboot

import "github.com/gin-gonic/gin"

type Route struct {
	Method     string
	Path       string
	Handler    gin.HandlerFunc
	Middleware []gin.HandlerFunc
}

type Controller interface {
	Routes() []Route
}

type RouterGroup struct {
	Path        string
	Middleware  []gin.HandlerFunc
	Controllers []Controller
}

func (s *Server) RegisterControllers(controllers ...Controller) {
	for _, controller := range controllers {
		for _, route := range controller.Routes() {
			handlers := route.Middleware
			handlers = append(handlers, route.Handler)

			switch route.Method {
			case "GET":
				s.engine.GET(route.Path, handlers...)
			case "POST":
				s.engine.POST(route.Path, handlers...)
			case "PUT":
				s.engine.PUT(route.Path, handlers...)
			case "DELETE":
				s.engine.DELETE(route.Path, handlers...)
			case "PATCH":
				s.engine.PATCH(route.Path, handlers...)
			case "OPTIONS":
				s.engine.OPTIONS(route.Path, handlers...)
			case "HEAD":
				s.engine.HEAD(route.Path, handlers...)
			}
		}
	}
}

func (s *Server) RegisterGroups(groups ...RouterGroup) {
	for _, group := range groups {
		routerGroup := s.engine.Group(group.Path)

		if len(group.Middleware) > 0 {
			routerGroup.Use(group.Middleware...)
		}

		for _, controller := range group.Controllers {
			for _, route := range controller.Routes() {
				handlers := route.Middleware
				handlers = append(handlers, route.Handler)

				switch route.Method {
				case "GET":
					routerGroup.GET(route.Path, handlers...)
				case "POST":
					routerGroup.POST(route.Path, handlers...)
				case "PUT":
					routerGroup.PUT(route.Path, handlers...)
				case "DELETE":
					routerGroup.DELETE(route.Path, handlers...)
				case "PATCH":
					routerGroup.PATCH(route.Path, handlers...)
				case "OPTIONS":
					routerGroup.OPTIONS(route.Path, handlers...)
				case "HEAD":
					routerGroup.HEAD(route.Path, handlers...)
				}
			}
		}
	}
}
