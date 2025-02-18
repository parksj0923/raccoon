# Raccoon Auto Trading Bot

Raccoon은 [Upbit](https://upbit.com) 등 거래소 API를 활용하여 자동으로 암호화폐 거래를 수행하는 Go 언어 기반의 자동매매 프로그램입니다.

---

## 주요 기능

- **실시간 데이터 피드 및 시세 구독**
    - 거래소(Upbit 등)와 연동하여 실시간 캔들 데이터 및 주문 이벤트 수신
    - 데이터 피드, 주문 피드를 별도의 구독 채널로 관리

- **다양한 전략 및 지표 지원**
    - 커스텀 전략을 통한 매매 신호 생성
    - EMA, RSI, MACD, Bollinger Bands, ADX, ATR, OBV, MFI, CCI, StochRSI 등 [TA-Lib](https://github.com/markcheno/go-talib) 기반의 다양한 기술적 지표 계산
    - 월별 추세 분석 등 커스텀 지표도 포함

- **모의투자 및 백테스팅**
    - 백테스트를 위한 `BackTestBroker` 구현
    - 지정 기간의 과거 캔들 데이터를 로드하여 전략 시뮬레이션 가능

- **웹 기반 모니터링**
    - SSE(Server-Sent Events)를 이용하여 실시간 캔들, 지표, 주문 이벤트 전송
    - Chart.js, Chart.js Financial 플러그인을 활용한 웹 차트 제공 (가격, 거래량, 주문, 지표)

- **알림 기능**
    - Telegram을 통한 주문 실행 결과(성공/실패) 알림 전송

- **모듈화된 아키텍처**
    - Exchange, Feed, Indicator, Strategy, WebServer 등 각 기능별로 모듈화된 패키지 구성
    - 인터페이스를 통해 실제 거래소 API와 모의 거래(MockExchange) 모두를 지원

---

## 프로젝트 구조

```plaintext
Raccoon/
├── exchange/           # 거래소 연동 관련 코드 (Upbit, BackTestBroker 등)
├── feed/               # 실시간 시세, 주문 피드 구독 및 퍼블리시 기능
├── consumer/           # 피드에서 전달된 캔들 및 주문 데이터를 소비(처리)하는 로직을 포함  
│                        - 전략 컨트롤러, 웹서버 등으로 데이터를 전달하여 후속 처리를 진행
├── indicator/          # 기술적 지표 계산 (go-talib 기반)
├── strategy/           # 자동매매 전략 및 전략 컨트롤러 (ImprovedPSHStrategy 등)
├── webserver/          # 웹 차트, SSE 서버 등 모니터링 기능
├── mocks/              # 인터페이스(MockExchange 등) 테스트용 모의 구현체
├── model/              # 데이터 구조체 (Candle, Order, Account, 등)
├── notification/       # Telegram 알림 기능 구현
├── utils/              # 유틸리티(로깅, 에러 처리, 기타 도구)
└── main.go             # 백테스트 실행 또는 봇 실행 진입점
```

## 실행방법
**환경변수 설정**
```shell
 export UPBIT_ACCESS_KEY=your_upbit_access_key
 export UPBIT_SECRET_KEY=your_upbit_secret_key
 export TELEGRAM_BOT_TOKEN=your_telegram_bot_token
 export TELEGRAM_CHAT_ID=your_telegram_chat_id
```
**실행**
```shell
 go build -o raccoon
 ./raccoon
```



