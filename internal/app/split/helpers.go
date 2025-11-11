package split

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Hackathon-Apps/go-split-api/internal/app/storage"
	"github.com/google/uuid"
)

func uuidFromVars(vars map[string]string, key string) (uuid.UUID, error) {
	idStr, ok := vars[key]
	if !ok || idStr == "" {
		return uuid.Nil, errors.New("missing id")
	}
	return uuid.Parse(idStr)
}

func parseOpType(s string) (storage.OpType, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case string(storage.OpContribute):
		return storage.OpContribute, nil
	case string(storage.OpTransfer):
		return storage.OpTransfer, nil
	case string(storage.OpRefund):
		return storage.OpRefund, nil
	default:
		return "", errors.New("invalid op_type: use CONTRIBUTE|TRANSFER|REFUND")
	}
}

func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
}

func renderJSON(w http.ResponseWriter, v interface{}) {
	js, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(js)
}

func renderErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Sender-Address")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) walletFromHeader(r *http.Request) (string, error) {
	c := r.Header.Get("Sender-Address")
	w := strings.TrimSpace(c)
	if w == "" {
		return "", errors.New("empty wallet header")
	}
	if len(w) < 36 {
		return "", errors.New("invalid wallet header")
	}
	return w, nil
}
