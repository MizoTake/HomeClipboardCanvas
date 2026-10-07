# Home Clipboard Canvas 仕様書 v0.1

## 1. 概要

家庭内LANで使用する、一時的な共有クリップボード／共有キャンバス。

PC・スマートフォン・タブレットなどからブラウザでアクセスし、テキストや画像をキャンバスへ貼り付ける。

貼り付けた内容はLAN内の他端末へリアルタイム同期され、別端末からコピーして取り出せる。

データは原則RAM上だけに保持し、一定時間経過後またはサーバー終了時に消去する。

### 主な目的

- PC間のテキスト受け渡し
- PC → スマートフォンへのURL共有
- スマートフォン → PCへのテキスト共有
- スクリーンショットや画像の一時共有
- 一時的なコマンド・コード断片の共有
- ファイルサーバーやクラウドを使うほどではないデータの受け渡し

---

# 2. プロジェクト名

仮称：

`HomeClipboardCanvas`

Git管理する。

```text
HomeClipboardCanvas
├─ .git\
├─ .gitignore
├─ README.md
├─ docs\
├─ backend\
├─ frontend\
├─ scripts\
└─ dist\
```

最終成果物：

```text
dist\
└─ HomeClipboardCanvas.exe
```

原則としてWindows上では、このEXE単体からサーバーを起動できる構成とする。

---

# 3. 開発PCに対する制約

今回開発に使用するPCは本番サーバーとして使用しない。

そのため、開発・ビルド作業では以下を行わない。

- Windowsスタートアップ登録
- レジストリ変更
- Windowsサービス登録
- Windows Firewallルール追加
- 証明書ストアへの証明書インストール
- hostsファイル変更
- DNS設定変更
- 常駐プロセス化
- Windows起動時の自動実行
- `%ProgramData%` 等へのアプリ設定保存

開発PCに残してよいものは、プロジェクトフォルダ内のファイルと、Git/Go/Node等の開発環境が生成する通常のキャッシュに限る。

サーバー向けセットアップ機能は実装するが、開発PCでは実行しない。

---

# 4. 基本構成

## Backend

Goを使用する。

理由：

- Windows向け単一EXEを生成しやすい
- HTTP/HTTPSサーバーを内蔵できる
- WebSocketを扱いやすい
- メモリ管理が容易
- Windowsサービス等へ将来発展可能
- ランタイムインストール不要

## Frontend

React + TypeScript + Vite。

キャンバス描画：

`react-konva / Konva`

を第一候補とする。

必要機能を限定し、Canvaやホワイトボードアプリほど多機能にはしない。

## 配布

FrontendをViteでビルドする。

生成された静的ファイルをGoの

```go
//go:embed
```

でEXE内部へ埋め込む。

最終的に、

```text
HomeClipboardCanvas.exe
```

だけでWeb UIとサーバーを提供できる状態にする。

---

# 5. 基本アーキテクチャ

```text
┌───────────────────┐
│ Windows PC        │
│ Chrome / Edge     │
└─────────┬─────────┘
          │
          │ HTTPS + WebSocket
          │
┌─────────▼─────────┐
│                   │
│ HomeClipboard     │
│ Canvas.exe        │
│                   │
│ ┌───────────────┐ │
│ │ Web Server    │ │
│ └───────────────┘ │
│                   │
│ ┌───────────────┐ │
│ │ WebSocket Hub │ │
│ └───────────────┘ │
│                   │
│ ┌───────────────┐ │
│ │ In-Memory     │ │
│ │ Board Store   │ │
│ └───────────────┘ │
│                   │
└─────────┬─────────┘
          │
          ├──────── PC
          ├──────── Smartphone
          └──────── Tablet
```

データベースは使用しない。

---

# 6. 画面仕様

基本画面は1枚の無限キャンバス。

```text
┌──────────────────────────────────────────────┐
│ Home Clipboard Canvas                   ●LAN │
├──────────────────────────────────────────────┤
│                                              │
│    ┌──────────────────────┐                  │
│    │ npm install foo      │                  │
│    │                      │                  │
│    │ Copy             ×   │                  │
│    └──────────────────────┘                  │
│                                              │
│                    ┌─────────────────┐       │
│                    │                 │       │
│                    │      IMAGE      │       │
│                    │                 │       │
│                    │ Copy        ×   │       │
│                    └─────────────────┘       │
│                                              │
│                                              │
└──────────────────────────────────────────────┘
```

キャンバスは、

- パン
- ズーム
- カード移動
- カードサイズ変更

に対応する。

---

# 7. Paste

キャンバス上で

```text
Ctrl + V
```

を行う。

Clipboard APIまたはpaste eventから内容を取得する。

## Text

文字列が存在した場合：

```text
Clipboard
↓
TextCard
```

としてキャンバスへ追加。

## Image

Clipboardに画像が存在した場合：

```text
Clipboard
↓
Image Blob
↓
ImageCard
```

として追加。

スクリーンショットツールから直接、

```text
PrintScreen
↓
Ctrl + V
```

も可能とする。

## Text + Image

クリップボードに複数形式が含まれる場合は、

1. Image
2. Plain Text

の優先順位とする。

将来的に設定可能としてもよい。

---

# 8. カード

## TextCard

保持情報：

```text
id
text
x
y
width
height
createdAt
updatedAt
expiresAt
```

UI：

```text
┌────────────────────────────┐
│ テキスト                   │
│                            │
│                            │
│ 12 min       Copy      ×   │
└────────────────────────────┘
```

### Copy

Copyボタンを押すと、

```text
navigator.clipboard.writeText()
```

でローカル端末のClipboardへコピーする。

---

# 9. ImageCard

保持情報：

```text
id
imageId
mimeType
x
y
width
height
createdAt
updatedAt
expiresAt
```

画像バイナリはサーバーRAMに保持する。

```text
Map<ImageID, []byte>
```

相当の構造を利用する。

### Copy Image

ブラウザが対応している場合、

```text
navigator.clipboard.write()
```

で画像をClipboardへコピーする。

対応していない場合：

```text
Open
Download
```

をフォールバックとして提供する。

---

# 10. リアルタイム同期

WebSocketを使用する。

イベント例：

```text
item.created
item.updated
item.deleted
item.expired
viewport.updated
board.cleared
```

端末A：

```text
Paste
↓
POST / API
↓
Memory Store
↓
WebSocket Broadcast
```

端末B：

```text
WebSocket
↓
即時Canvas更新
```

原則としてページリロードは不要。

---

# 11. Viewport共有

キャンバス表示状態：

```text
x
y
zoom
updatedAt
```

をサーバーRAM上に保持する。

目的：

```text
PC Aで作業
↓
最後に表示していた場所を保存
↓
PC Bでアクセス
↓
同じ付近から表示
```

## 更新条件

単純にすべてのスクロールを共有すると他端末の画面まで動いてしまうため、リアルタイム同期はしない。

各クライアントはローカルでViewportを保持する。

以下のタイミングでサーバーへ送信する。

```text
visibilitychange
pagehide
beforeunload
```

または操作終了後一定時間経過。

ただし、最後にユーザー操作が行われたViewportのみ採用する。

サーバー側では、

```text
lastInteractionAt
```

を比較して最新のものを保持する。

---

# 12. 最新アイテム

以下もRAM上に保持する。

```text
lastCreatedItemId
lastCreatedAt
```

別端末からアクセスした場合、

通常：

```text
lastViewport
```

を復元。

ただし、前回利用時以降に新しいアイテムが追加されている場合は、

```text
最新アイテムへ移動
```

できるボタンを表示する。

例：

```text
[ New item ↓ ]
```

勝手にViewportを移動させる仕様にはしない。

---

# 13. TTL

デフォルト：

```text
1 hour
```

アイテム作成時：

```text
expiresAt = createdAt + 1h
```

とする。

一定間隔でサーバーが期限切れを検出する。

推奨：

```text
30秒間隔
```

期限切れ時：

```text
Memoryから削除
↓
全クライアントへitem.expired
```

画像バイナリも同時にRAMから解放する。

---

# 14. 永続化

Canvas内容については一切永続化しない。

保存しないもの：

- テキスト
- 画像
- Canvas位置
- Clipboard履歴
- Item履歴
- lastViewport

サーバーを終了するとすべて消える。

```text
EXE終了
↓
RAM解放
↓
データ消滅
```

これを仕様とする。

---

# 15. メモリ制限

暴走防止のため制限を設ける。

初期値：

```text
最大Item数       100
最大Imageサイズ   10 MB
最大Textサイズ     1 MB
Canvas全体上限   200 MB
TTL              60 min
```

上限を超える場合はアップロードを拒否する。

HTTP：

```text
413 Payload Too Large
```

相当を返す。

---

# 16. 画像最適化

ブラウザから送信する前に必要に応じて画像を縮小可能とする。

初期仕様：

```text
最大辺 2560px
```

を超える場合のみ縮小。

ただしClipboard内容をできるだけ変更しない方針から、MVPでは元画像を保持する実装でもよい。

画像圧縮はPhase 2候補。

---

# 17. LAN限定

外部公開を前提としない。

Backend側でもアクセス元IPをチェックする。

デフォルト許可：

```text
127.0.0.0/8
10.0.0.0/8
172.16.0.0/12
192.168.0.0/16
::1
fc00::/7
```

これ以外：

```text
403 Forbidden
```

とする。

設定によって追加CIDRを許可可能にする。

例：

```text
100.64.0.0/10
```

Tailscale対応を将来的に可能とする。

---

# 18. HTTPS

HTTPではなくHTTPSを基本とする。

サーバーはTLSを直接扱える構造にする。

設定：

```text
certificate.pem
private-key.pem
```

を指定可能。

例：

```text
HomeClipboardCanvas.exe serve ^
  --cert certificate.pem ^
  --key private-key.pem
```

証明書が設定されていない場合は起動を拒否するモードをデフォルトとする。

開発時のみ、

```text
--dev-http
```

を指定すればlocalhost HTTP起動可能。

```text
http://127.0.0.1:8080
```

ただしLAN公開は不可。

---

# 19. 認証

LAN内限定でも、URLを知っているだけでアクセス可能にはしない。

シンプルな共有シークレットを使用する。

サーバー側：

```text
32byte random token
```

初回アクセス：

```text
https://clip.example/?token=xxxxx
```

認証成功後：

```text
Secure
HttpOnly
SameSite=Strict
```

Cookieを発行する。

その後URLからtokenを除去する。

WebSocketにも同じセッション認証を使用する。

---

# 20. セキュリティ

最低限以下を実装する。

HTTP Header：

```text
Content-Security-Policy
X-Content-Type-Options: nosniff
Referrer-Policy: no-referrer
X-Frame-Options: DENY
```

さらに：

- SVGアップロード禁止
- HTMLをCanvasへ直接描画しない
- TextCardはplain textとして扱う
- ファイルパスをユーザー入力から生成しない
- LAN外IP拒否
- Request size limit
- WebSocket origin check
- Token authentication
- HTTPS mandatory
- Rate limit
- MIME検証

テキストはReact上でHTMLとして解釈しない。

```text
dangerouslySetInnerHTML
```

は使用禁止。

---

# 21. 設定

Canvasデータは永続化しないが、サーバーとして使用するPCでは起動設定だけ保存できるようにする。

例：

```json
{
  "port": 8443,
  "ttlMinutes": 60,
  "maxItems": 100,
  "maxMemoryMB": 200,
  "allowedNetworks": [
    "192.168.0.0/16"
  ]
}
```

実サーバーPC：

```text
%ProgramData%\HomeClipboardCanvas\
```

への保存を候補とする。

ただしportableモード：

```text
HomeClipboardCanvas.exe
config.json
certificate.pem
private-key.pem
```

も可能にする。

---

# 22. 開発PCでは設定を残さない

開発PCでは、

```text
--install
--install-startup
--install-firewall
--install-cert
```

等は実行しない。

テスト時：

```text
HomeClipboardCanvas.exe serve --dev-http --bind 127.0.0.1
```

のみ使用可能。

つまりネットワーク上にも公開しない。

終了すれば状態はすべて消える。

---

# 23. EXE CLI

```text
HomeClipboardCanvas.exe serve
```

通常起動。

---

```text
HomeClipboardCanvas.exe version
```

バージョン表示。

---

```text
HomeClipboardCanvas.exe generate-token
```

ランダム共有Token生成。

---

```text
HomeClipboardCanvas.exe install-startup
```

スタートアップ登録。

---

```text
HomeClipboardCanvas.exe uninstall-startup
```

スタートアップ解除。

---

```text
HomeClipboardCanvas.exe status
```

設定状態確認。

---

# 24. Windowsスタートアップ

Windowsサービスではなく、MVPではユーザースタートアップとして実装する。

登録先：

```text
%APPDATA%\Microsoft\Windows\Start Menu\Programs\Startup
```

ここへShortcutを作成する。

起動コマンド：

```text
HomeClipboardCanvas.exe serve
```

## 登録

```text
HomeClipboardCanvas.exe install-startup
```

## 解除

```text
HomeClipboardCanvas.exe uninstall-startup
```

登録前に現在の登録状態を確認する。

重複登録しない。

---

# 25. 開発PCではStartup登録禁止

今回の作業では、

```text
install-startup
```

のコード実装とテスト可能性確認まで行う。

実際のスタートアップフォルダへの登録は行わない。

必要であればUnit TestではTemporary Directoryを使用してShortcut生成ロジックのみ確認する。

---

# 26. ログ

Clipboard内容そのものはログへ出力しない。

NG：

```text
received text: AWS_SECRET_KEY=...
```

OK：

```text
item created
type=text
size=421
client=192.168.x.x
```

ログレベル：

```text
ERROR
WARN
INFO
DEBUG
```

デフォルト：

```text
INFO
```

---

# 27. UI上の端末表示

接続端末数だけ表示する。

```text
● 3 devices
```

端末名・Windowsユーザー名等はMVPでは取得しない。

プライバシーと実装簡略化のため。

---

# 28. Clear

全削除ボタンを設ける。

```text
Clear Canvas
```

誤操作防止のため確認：

```text
Clear all temporary items?
```

実行：

```text
RAM上の全Item削除
RAM上の全Image削除
WebSocketで全端末へ通知
```

---

# 29. UI操作

Desktop：

```text
左ドラッグ     カード移動
背景ドラッグ   Canvas移動
Wheel          Zoom
Ctrl+V         Paste
Delete         選択削除
Ctrl+C         選択内容Copy
```

Mobile：

```text
1本指           操作
2本指           Pan / Zoom
Tap             選択
Long press       Copy menu
```

---

# 30. MVP対象外

以下は最初は実装しない。

- ファイル共有
- 動画共有
- 音声共有
- ユーザーアカウント
- 複数Canvas
- 永続履歴
- Markdownエディタ
- 図形作成
- 描画ツール
- コメント
- チャット
- OCR
- AI連携
- Internet公開
- Cloud Sync
- 自動Clipboard監視
- OSレベルClipboard同期

---

# 31. Phase 2候補

必要になった場合のみ追加する。

### FileCard

ファイルを一時共有。

### QR

スマートフォンからCanvasへ簡単にアクセス。

### PWA

ホーム画面追加。

### Tailscale

自宅LAN外から安全にアクセス。

### Send to Device

特定端末にItemを送る。

### Clipboard Agent

Windows常駐アプリによる明示的な

```text
Send Clipboard
```

機能。

Clipboard自動監視はデフォルト無効。

---

# 32. Git

初期ブランチ：

```text
main
```

.gitignore：

```text
dist/
frontend/node_modules/
frontend/dist/
*.exe
*.pem
*.key
config.json
.env
```

秘密鍵・Token・証明書・実設定はGitへコミットしない。

---

# 33. Build

例：

```text
scripts\
├─ build.ps1
├─ clean.ps1
└─ dev.ps1
```

## build.ps1

処理：

```text
npm ci
↓
npm run build
↓
frontend/dist生成
↓
Go embed
↓
go build
↓
dist/HomeClipboardCanvas.exe
```

最終確認：

```text
dist\
└─ HomeClipboardCanvas.exe
```

---

# 34. Development

開発起動：

```powershell
.\scripts\dev.ps1
```

Backend：

```text
127.0.0.1
```

だけにbind。

LANへ公開しない。

Frontendの開発サーバーもlocalhost限定。

---

# 35. テスト要件

最低限以下を確認する。

### Backend

- Text item追加
- Image item追加
- Item削除
- TTL削除
- メモリ上限
- Item数上限
- LAN IP判定
- Token認証
- WebSocket接続
- WebSocket broadcast
- Clear
- Startup shortcut生成ロジック

### Frontend

- Text Paste
- Image Paste
- Text Copy
- Image Copy
- Drag
- Resize
- Pan
- Zoom
- WebSocket同期
- Expire反映
- Reconnect

---

# 36. 完成条件

MVP完成と判断する条件：

1. プロジェクトフォルダにGit Repositoryが存在する
2. `main` ブランチが存在する
3. `README.md` が存在する
4. WindowsでBuild可能
5. `dist/HomeClipboardCanvas.exe` が生成される
6. EXE単体でWebサーバーが起動する
7. localhostからCanvasを開ける
8. TextをPasteできる
9. ImageをPasteできる
10. Copyできる
11. 複数ブラウザ間でリアルタイム同期できる
12. Itemが1時間で消える
13. サーバー再起動後にItemが復元されない
14. LAN制限が動作する
15. HTTPSに対応する
16. Token認証が動作する
17. Startup登録機能が実装されている
18. Startup解除機能が実装されている
19. 開発PCのStartupには実際には登録されていない
20. 開発PCの証明書・Firewall・Registryを変更していない

---

# 37. 最終的な利用イメージ

実際にサーバーにするWindows PCへ、

```text
HomeClipboardCanvas.exe
config.json
certificate.pem
private-key.pem
```

を配置。

スタートアップ登録：

```powershell
HomeClipboardCanvas.exe install-startup
```

以降Windowsログイン時に自動起動。

各端末：

```text
https://clip.home.arpa
```

へアクセス。

PCで：

```text
Ctrl+C
↓
Browser
↓
Ctrl+V
```

スマートフォンから同じCanvasを開いて：

```text
Tap
↓
Copy
```

逆方向も同様。

サーバー上の情報はRAMだけに存在し、

```text
1時間
```

経過またはEXE終了で消去される。

Home Clipboard Canvasは、

**「家庭内に1枚だけ存在する、揮発性の共有クリップボードキャンバス」**

として実装する。