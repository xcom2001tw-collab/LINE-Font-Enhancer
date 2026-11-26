LINE Font Enhancer V2.0 RC5（介面精簡版）
非 LINE 官方工具。Windows x64，無額外下載、無 Qt SDK 相依。

執行 EXE，從下拉選單選擇字體及放大比例。
圖片順序已由實測者確認並固定：左側原廠，右側 FreeType。忽略 RC1 本機標記檔。
樣本順序：
微軟正黑體 A=061041 B=061155
金萱粉圓體 A=060729 B=060911
標楷體 A=061242 B=061316
新細明體 A=061408 B=061440
Noto Sans TC Medium A=061746 B=061827
圖片僅裁切原始測試文字，不做銳化或人工美化；完整樣本在空間不足時使用兩側相同縮放比例。100/200/400% 是原始像素倍率，超出面板以左上局部顯示。

試用與套用會要求確認關閉 LINE，若仍在背景則再次確認強制結束。請先完成通話、傳檔、草稿。
套用只對 LINE 子程序設定 QT_QPA_PLATFORM=windows:fontengine=freetype，建立桌面捷徑和選用登入啟動捷徑。自啟停用僅支援可辨識的目前使用者 Run / Startup；保留 V1.3–V1.7 備份格式，V1.1 XML 備份仍須由 V1.1 還原。
還原會移除工具捷徑，恢復可還原的原廠自啟項目，並重新啟動原廠 LINE。啟動主介面後在背景嘗試清理 V1.6 專用快取；偵測到相關程序、特殊連結或錯誤時略過。保留設定及備份。

編譯（Go 1.20+，Windows x64 或交叉編譯環境）：
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 GO111MODULE=off go build -trimpath -ldflags '-H windowsgui -s -w' -o LINE_Font_Enhancer_V2.0_RC5.exe .

本版已交叉編譯並確認 PE32+ x64 GUI 格式，未在 Windows 實機測試。未簽章。請先實機測試。

RC3：所有圖片啟動時解碼，使用 Go 像素緩衝區及 WM_PAINT / StretchDIBits，移除 HBITMAP 替換流程。診斷：%TEMP%\LINE_Font_Enhancer_V20_RC5.log。

RC5：綠白配色、圓角按鈕、主操作強調、三個主要按鈕、背景清理、診斷紀錄容量限制、內嵌 EXE 圖示。
