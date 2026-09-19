package tunnel

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

const origin = "http://127.0.0.1:7676"

var quickURL = regexp.MustCompile(`https://[a-z0-9]+(?:-[a-z0-9]+)*\.trycloudflare\.com`)

// Quick possui exclusivamente o processo cloudflared iniciado pelo SignalSpace.
// Nenhum comando é montado a partir de uma URL ou de saída não confiável.
type Quick struct {
	URL      string
	cmd      *exec.Cmd
	done     chan struct{}
	mu       sync.Mutex
	waitErr  error
	stopOnce sync.Once
}

// Start inicia um Quick Tunnel e espera por uma URL estritamente trycloudflare.com.
// A URL não indica que o servidor de origem ou o OAuth estejam funcionando.
func Start(ctx context.Context) (*Quick, error) {
	binary, err := exec.LookPath("cloudflared")
	if err != nil {
		return nil, errors.New("cloudflared not found in PATH; install it before using connect quick")
	}
	return start(ctx, binary, 30*time.Second)
}

func start(ctx context.Context, binary string, timeout time.Duration) (*Quick, error) {
	cmd := exec.CommandContext(ctx, binary, "tunnel", "--url", origin)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start cloudflared: %w", err)
	}
	q := &Quick{cmd: cmd, done: make(chan struct{})}
	found := make(chan string, 1)
	var readers sync.WaitGroup
	readers.Add(2)
	for _, stream := range []io.Reader{stdout, stderr} {
		go func(stream io.Reader) {
			defer readers.Done()
			scanner := bufio.NewScanner(stream)
			// Saídas extensas ou binárias não podem consumir memória sem limite.
			scanner.Buffer(make([]byte, 4096), 64<<10)
			for scanner.Scan() {
				if host := extractQuickURL(scanner.Text()); host != "" {
					select {
					case found <- host:
					default:
					}
				}
			}
		}(stream)
	}
	go func() {
		// Consumir ambos os pipes antes de Wait evita o fechamento prematuro
		// documentado por os/exec e não perde a URL em processos curtos.
		readers.Wait()
		err := cmd.Wait()
		q.mu.Lock()
		q.waitErr = err
		q.mu.Unlock()
		close(q.done)
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case url := <-found:
		select {
		case <-q.done:
			_ = q.Close()
			return nil, errors.New("cloudflared exited before the tunnel could be used")
		default:
		}
		q.URL = url
		return q, nil
	case <-q.done:
		return nil, errors.New("cloudflared exited without providing a Quick Tunnel URL; check its installation and ~/.cloudflared/config.yaml")
	case <-timer.C:
		_ = q.Close()
		return nil, errors.New("cloudflared did not provide a Quick Tunnel URL before the timeout")
	case <-ctx.Done():
		_ = q.Close()
		return nil, ctx.Err()
	}
}

func extractQuickURL(line string) string {
	for _, match := range quickURL.FindAllStringIndex(line, -1) {
		// Evitar URLs parciais dentro de outros domínios, caminhos ou credenciais.
		if match[0] > 0 && (isHostChar(line[match[0]-1]) || line[match[0]-1] == '@') {
			continue
		}
		if match[1] < len(line) && (isHostChar(line[match[1]]) || strings.ContainsRune("/:@", rune(line[match[1]]))) {
			continue
		}
		return line[match[0]:match[1]]
	}
	return ""
}

func isHostChar(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '.' || b == '-' || b == '_'
}

// Done fecha quando o processo termina, inclusive após encerramento solicitado.
func (q *Quick) Done() <-chan struct{} { return q.done }

// Err devolve somente a situação do processo; não expõe seus logs potencialmente sensíveis.
func (q *Quick) Err() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.waitErr
}

// Close solicita o encerramento e espera a confirmação do processo filho.
func (q *Quick) Close() error {
	q.stopOnce.Do(func() {
		if q.cmd.Process != nil {
			_ = q.cmd.Process.Kill()
		}
	})
	<-q.done
	return nil
}
