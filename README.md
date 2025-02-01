# raccoon


# 기능 

WarmupPeriod Preload

SetupSubscriptions()에서 
    strat.Timeframe()
    strat.WarmupPeriod()를 통해 계산한 뒤 
    exchange.CandlesByPeriod(..., start, end)로 불러오고
    dataFeedSub.Preload(...)에 넣음.
프레임워크 구독자(Controller) 입장에서는 이미 완료된 과거 봉들을 한꺼번에 먼저 받게 되므로, \ WarmupPeriod()개 이상 로딩 시 바로 지표가 정상 계산 가능 → 굿.
실시간봉(WebSocket) 수신 → 부분봉(Complete=false) + 완성봉(Complete=true) 둘 다 Emit

handleCandle1s에서 push1sCandle 호출
partial, final, isFinal := agg.push1sCandle(candle)
agg.candleCh <- partial (if partial.Volume > 0)
agg.candleCh <- final (if isFinal && final.Volume > 0)
이로써 DataFeed 측에서 onCandleClose=false 구독자는 부분봉도 받고, onCandleClose=true 구독자는 완성봉만 받음.
(현재 전략 컨트롤러는 완성봉만 사용 → onCandleClose=true)

Strategy(PSHStrategy)

Indicators()에서 월별추세, EMA, RSI, Bollinger, MACD 등을 계산 → df.Metadata[...]에 저장.
OnCandle에서 “50봉(혹은 60봉) 이후부터 매매” / 단순 시그널 / OrderFeed.Publish


OrderFeed

실제 Broker 체결 로직은 OrderFeedConsumerBroker.OnOrder
메시지큐-like 구조로, 전략과 체결 로직을 분리

Controller

Dataframe 누적 / WarmupPeriod 길이 만족 시 → Strategy.Indicators(...) + (started=true이면) → Strategy.OnCandle(...)

프로그램 시작
Preload(최소 WarmupPeriod만큼 과거 봉) → Controller가 이미 일정 봉수 있는 상태로 지표 계산
WS 연결 + 부분봉/완성봉 발생 → DataFeed → (onCandleClose=true) → Controller → Strategy → (주문시) OrderFeed.Publish
OrderFeed → Consumer → Broker 체결

# 구조
raccoon.go (메인 봇)

Exchange(Upbit), DataFeed, OrderFeed, Strategy, Controller를 내부에서 관리
SetupSubscriptions()에서 Preload + Subscribe 설정
Start()/Stop()에서 DataFeedSub, OrderFeedSub, Exchange 모두 구동/종료

consumer 패키지
DataFeedConsumer(Controller.OnCandle로 전달) / OrderFeedConsumerBroker(Broker CreateOrder*로 체결)

feed.DataFeedSubscription
onCandleClose 옵션
Preload 시 이미 Complete=true 봉만 한꺼번에 전달
부분봉(Complete=false) + 완성봉(Complete=true)을 받되, 구독자가 “complete=true만 원하면 해당 봉만” 호출 → \ 단일 채널 구조지만, boolean 분기

CandleAggregator
1초봉 -> 부분봉(Partial) / 주기 경계 시 완성봉(Final)
buffer에서 removeOldSeconds, aggregateBuffer 등 → 중복/누락 관리.

nohup caffeinate -i ./raccoon > raccoon.log 2>&1 &

