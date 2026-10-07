# 設計

spec.md v0.1のMVPを対象にしています。LAN内のURLへの直接アクセスで利用し、トークン認証は使用しません。Phase 2機能やインターネットへのサーバー公開は含みません。

## 構成と選択理由

| 部分 | 採用 | 理由と代替 |
| --- | --- | --- |
| Backend | Go、標準net/http、go:embed | Web UIを含む単体EXEにし、OS設定なしで起動できる |
| 同期 | coder/websocket v1.8.15 | WebSocketのフレーム処理、ping、切断、Originチェックを既存実装へ委ねる。標準ライブラリでの自作はプロトコル保守の負担が大きい |
| Frontend | React 19.3、TypeScript 7、Vite 8.3 | 仕様の構成に沿う。バージョンはlockfileに固定 |
| カード | HTML＋CSS transform | テキストの選択、semantic HTML、通常のコピー・保存ボタン、キーボード操作を提供しやすい。Konvaは仕様の第一候補だが、今回は描画ツールを扱わないため採用していない。ユーザーから安全性・分かりやすさを優先する回答を得て選択 |
| ブラウザ検証 | Playwright 1.63 | 実際のEXEとChromiumで複数接続、Clipboard、タッチ操作を再現する。追加依存なしの手作業だけでは繰り返し検証が難しい |

画像の再圧縮は行いません。CSS transformの合成はブラウザに委ねます。GPU依存のサーバー処理や追加ランタイムは不要です。

参照: [Go embed](https://pkg.go.dev/embed)、[coder/websocket](https://pkg.go.dev/github.com/coder/websocket)、[KonvaのDOM要素の扱い](https://konvajs.org/docs/react/DOM_Portal.html)。外部フォントやアイコン・アニメーションの依存は追加していません。

## データと同期

`board.Store` がmutexでカード・画像・表示位置・クライアントを保護します。作成・更新・削除と通知の投入を同じlockの中で行います。接続時には、購読登録とsnapshot取得を同じlock内で行い、snapshot取得と通知開始の隙間をなくします。

最大64接続、接続ごとの通知キューは128件です。遅い接続は切断して、再接続時に完全なsnapshotを送り直します。カードにはrevisionを付け、古いrevisionでの変更を409として最新カードを返します。位置は操作終了時に一度だけ送信し、本文や期限は更新APIから変更できません。

クライアントのカード状態はWebSocketの順序付き通知と接続時のsnapshotから更新します。HTTP応答からカードを再挿入しないため、変更応答が削除通知より遅れて到着しても削除済みカードは復元されません。

表示位置はWebSocketでは配信しません。クライアントの時計はsnapshotのserverTimeとの差を補正し、lastInteractionAtを比較して古い終了通知が新しい表示を上書きしないようにします。極端な未来時刻も拒否します。接続時にだけ保存位置を復元し、再接続snapshotでは現在の表示位置を維持します。

切断中の操作は無効にし、未同期の内容をlocalStorageへ保存しません。再接続は1〜15秒のbackoff、OSのoffline/online通知でも切断・再接続します。サーバーの再起動後もログインやページ再読み込みなしで再接続し、空のsnapshotへ更新します。

## API

| メソッド | パス | 内容 |
| --- | --- | --- |
| GET | /health | LAN内の起動確認、204。状態やデータは返さない |
| GET | /api/board | items、viewport、latest、limits、devices、serverTimeのsnapshot |
| POST | /api/items | JSON `{text,x,y,width,height}`。作成して201 |
| POST | /api/images?x=…&y=…&width=…&height=… | 生の画像バイナリ。Content-Typeはimage/png・jpeg・gif |
| GET | /api/images/{imageId} | 元の画像バイナリ |
| PATCH | /api/items/{id} | JSON `{x,y,width,height,revision}` |
| DELETE | /api/items/{id} | カードと対応する画像を削除 |
| DELETE | /api/board | 全削除 |
| POST | /api/viewport | JSON `{x,y,zoom,lastInteractionAt,updatedAt?}` |
| GET | /api/ws | LAN接続元と同一Originを確認してWebSocketへupgrade |

通知は `board.snapshot`、`item.created`、`item.updated`、`item.deleted`、`item.expired`、`board.cleared`、`devices.updated` です。JSONは未知フィールド、複数オブジェクト、不正な座標を拒否します。

## セキュリティ境界

実接続のRemoteAddrを確認し、X-Forwarded-Forは信頼しません。標準のprivate/loopback CIDRと追加CIDRだけを許可します。開発HTTPはbindと接続元の両方をloopbackに限定します。実運用はTLS 1.2以上と正しい証明書・秘密鍵を要求します。

アクセス可否は接続元IPで判定します。許可したネットワーク内の端末はカードを閲覧・変更できます。共有トークン、認証用Cookie、ログインAPI、認証情報の環境変数は使いません。

更新API・WebSocketではscheme、Host、Originの一致を必須とします。CSP、nosniff、no-referrer、frame拒否、no-storeを設定します。HTMLは解釈せず、画像はMIME・実データ・サイズ・画素数を検証します。multipartやアップロード先ファイルは使いません。

IPごとのAPIレートは80件burst・毎秒4件補充です。レート情報は最大2,048キーに制限し、古いキーを回収します。同時カード作成処理はテキストと画像を合わせて2件までです。テキストのJSONエスケープによる増加はリクエスト上限に織り込み、復号後のUTF-8サイズでテキスト上限を判定します。

ディスクへ永続化できるのは、明示的に用意した起動設定と証明書だけです。実サーバー向けのProgramData自動保存や証明書・Firewallの自動設定は実装していません。MVPのportable構成では、EXE隣のconfig.jsonまたは --configを使います。
