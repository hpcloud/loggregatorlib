package handlers_test

import (
	"github.com/cloudfoundry/loggregatorlib/loggertesthelper"
	"github.com/cloudfoundry/loggregatorlib/server/handlers"
	"github.com/gorilla/websocket"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("WebsocketHandler", func() {
	var handler http.Handler
	var fakeResponseWriter *httptest.ResponseRecorder
	var messagesChan chan []byte
	var testServer *httptest.Server
	var handlerDone chan struct{}

	BeforeEach(func() {
		fakeResponseWriter = httptest.NewRecorder()
		messagesChan = make(chan []byte, 10)
		handler = handlers.NewWebsocketHandler(messagesChan, 100*time.Millisecond, loggertesthelper.Logger())
		handlerDone = make(chan struct{})
		testServer = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			handler.ServeHTTP(rw, r)
			close(handlerDone)
		}))
	})

	AfterEach(func() {
		testServer.Close()
	})

	It("should complete when the input channel is closed", func() {
		_, _, err := websocket.DefaultDialer.Dial(httpToWs(testServer.URL), nil)
		Expect(err).NotTo(HaveOccurred())
		close(messagesChan)
		Eventually(handlerDone).Should(BeClosed())
	})

	It("fowards messages from the messagesChan to the ws client", func() {
		for i := 0; i < 5; i++ {
			messagesChan <- []byte("message")
		}

		ws, _, err := websocket.DefaultDialer.Dial(httpToWs(testServer.URL), nil)
		Expect(err).NotTo(HaveOccurred())
		for i := 0; i < 5; i++ {
			msgType, msg, err := ws.ReadMessage()
			Expect(msgType).To(Equal(websocket.BinaryMessage))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(msg)).To(Equal("message"))
		}
		go ws.ReadMessage()
		close(messagesChan)
	})

	It("should err when websocket upgrade fails", func() {
		resp, err := http.Get(testServer.URL)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

	})

	It("should stop when the client goes away", func() {
		ws, _, err := websocket.DefaultDialer.Dial(httpToWs(testServer.URL), nil)
		Expect(err).NotTo(HaveOccurred())

		ws.Close()
		go func() {
			handlerDone, messagesChan := handlerDone, messagesChan
			for {
				select {
				case messagesChan <- []byte("message"):
				case <-handlerDone:
					return
				}
			}
		}()

		Eventually(handlerDone).Should(BeClosed())
	})

	It("should stop when the client goes away, even if no messages come", func() {
		ws, _, err := websocket.DefaultDialer.Dial(httpToWs(testServer.URL), nil)
		Expect(err).NotTo(HaveOccurred())

		//		ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Time{})
		ws.Close()

		Eventually(handlerDone).Should(BeClosed())
	})

	It("should stop when the client doesn't respond to pings", func() {
		ws, _, err := websocket.DefaultDialer.Dial(httpToWs(testServer.URL), nil)
		Expect(err).NotTo(HaveOccurred())

		ws.SetPingHandler(func(string) error { return nil })
		go ws.ReadMessage()

		Eventually(handlerDone).Should(BeClosed())
	})

	It("should continue when the client resonds to pings", func() {
		ws, _, err := websocket.DefaultDialer.Dial(httpToWs(testServer.URL), nil)
		Expect(err).NotTo(HaveOccurred())

		go ws.ReadMessage()

		Consistently(handlerDone, 200*time.Millisecond).ShouldNot(BeClosed())
		close(messagesChan)
	})

	It("should continue when the client sends old style keepalives", func() {
		ws, _, err := websocket.DefaultDialer.Dial(httpToWs(testServer.URL), nil)
		Expect(err).NotTo(HaveOccurred())

		go func() {
			for {
				ws.WriteMessage(websocket.TextMessage, []byte("I'm alive!"))
				time.Sleep(100 * time.Millisecond)
			}
		}()
		go ws.ReadMessage()

		Consistently(handlerDone, 200*time.Millisecond).ShouldNot(BeClosed())
		close(messagesChan)
	})

	It("should send a closing message", func() {
		ws, _, err := websocket.DefaultDialer.Dial(httpToWs(testServer.URL), nil)
		Expect(err).NotTo(HaveOccurred())
		close(messagesChan)
		_, _, err = ws.ReadMessage()
		Expect(err).To(Equal(io.EOF))
	})

})

func httpToWs(u string) string {
	return "ws" + u[len("http"):]
}
