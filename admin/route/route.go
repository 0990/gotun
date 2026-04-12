// Sword will check if route file is created, if existed, Sword will modify it
// If you want to recreated the route,you should delete the file,and then use Sword generate again, or you can copy from the `stub/route/route.stub` file

// Do not modify the notes `----Route-begin----` or `----Route-end----` or `----Import----`

package route

import (
	"compress/gzip"
	"embed"
	"encoding/json"
	"fmt"
	"github.com/0990/gotun/admin/controller/tunnel"
	"github.com/0990/gotun/admin/response"
	"github.com/0990/gotun/tun"
	"io/fs"
	"log"
	"net/http"
	// ----Import----
)

type gZipWriter struct {
	gz *gzip.Writer
	http.ResponseWriter
}

func (u *gZipWriter) Write(p []byte) (int, error) {
	return u.gz.Write(p)
}

func Register(assets embed.FS, listen string, mgr *tun.Manager, authMgr *AuthManager, version string) {
	h := http.NewServeMux()
	// Static file
	h.Handle("/go_sword_public/", http.StripPrefix("/go_sword_public/",
		http.FileServer(http.FS(assets))))
	h.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if !authMgr.IsEnabled() || authMgr.IsAuthenticated(r) {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(readFile(assets, "admin/view/login.html"))
	})
	h.HandleFunc("/api/auth/challenge", authMgr.HandleChallenge)
	h.HandleFunc("/api/auth/login", authMgr.HandleLogin)
	h.HandleFunc("/api/auth/logout", authMgr.HandleLogout)
	h.HandleFunc("/api/auth/session", authMgr.HandleSession)
	// Default index.html
	h.HandleFunc("/", authMgr.RequirePageAuth(func(w http.ResponseWriter, r *http.Request) {
		//Static file route
		fsys, _ := fs.Sub(assets, "admin/resource/dist")
		handle := http.FileServer(http.FS(fsys))
		//handle := http.FileServer(http.Dir("admin/resource/dist"))
		w.Header().Set("Content-Encoding", "gzip")

		gz := gzip.NewWriter(w)
		newWriter := &gZipWriter{
			gz:             gz,
			ResponseWriter: w,
		}
		defer gz.Close()
		handle.ServeHTTP(newWriter, r)
	}))
	// Render Vue html component
	h.HandleFunc("/render", authMgr.RequireAuth(renderWithAssets(assets)))
	// ----Route-begin----

	// Route tag tunnel
	h.HandleFunc("/api/tunnel/list", authMgr.RequireAuth(tunnel.List(mgr, version)))
	h.HandleFunc("/api/tunnel/delete", authMgr.RequireAuth(tunnel.Delete(mgr)))
	h.HandleFunc("/api/tunnel/create", authMgr.RequireAuth(tunnel.Create(mgr)))
	h.HandleFunc("/api/tunnel/edit", authMgr.RequireAuth(tunnel.Edit(mgr)))
	h.HandleFunc("/api/tunnel/import", authMgr.RequireAuth(tunnel.Import(mgr)))
	h.HandleFunc("/api/tunnel/export", authMgr.RequireAuth(tunnel.Export(mgr)))
	h.HandleFunc("/api/tunnel/check_server", authMgr.RequireAuth(tunnel.CheckServer(mgr)))
	h.HandleFunc("/api/tunnel/quality", authMgr.RequireAuth(tunnel.Quality(mgr)))
	h.HandleFunc("/api/tunnel/probe", authMgr.RequireAuth(tunnel.Probe(mgr)))
	h.HandleFunc("/api/tunnel/bandwidth", authMgr.RequireAuth(tunnel.Bandwidth(mgr)))
	h.HandleFunc("/api/tunnel/mtr/stream", authMgr.RequireAuth(tunnel.MTRStream(mgr)))
	// ----Route-end----

	go func() {
		err := http.ListenAndServe(listen, h)
		if err != nil {
			panic(err)
		}
	}()
}

func handleError(h func(w http.ResponseWriter, r *http.Request) error) func(w http.ResponseWriter,
	r *http.Request) {

	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				msg, _ := json.Marshal(response.Ret{
					Code: http.StatusInternalServerError,
					Msg:  fmt.Sprintf("%v", err),
				})

				log.Printf("%s", msg)
				w.Write(msg)
			}
		}()

		err := h(w, r)
		if err != nil {
			log.Printf("%v", err)
			msg, _ := json.Marshal(response.Ret{
				Code: http.StatusInternalServerError,
				Msg:  fmt.Sprintf("%v", err),
			})
			w.Write(msg)
		}
	}
}

func renderWithAssets(assets embed.FS) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		// 解析参数，映射到文件
		err := request.ParseForm()
		if err != nil {
			panic(err.Error())
		}

		path := request.FormValue("path")
		if path == "" {
			panic("lose path param")
		}

		// 从view目录中寻找文件
		body := readFile(assets, "admin/view"+path+".html")
		_, err = writer.Write(body)

		if err != nil {
			panic(err.Error())
		}
	}
}

func readFile(assets embed.FS, path string) []byte {
	body, err := assets.ReadFile(path)
	if err != nil {
		panic(err.Error())
	}

	return body
}
