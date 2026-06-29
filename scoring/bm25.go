package scoring

import "math"

// Params holds the two BM25 tuning constants.
type Params struct {
	K1 float64 // term frequency saturation (typically 1.2–2.0)
	B  float64 // length normalization (0 = off, 1 = full, typically 0.75)
}

// DefaultParams returns the standard BM25 tuning values used by Elasticsearch.
func DefaultParams() Params {
	return Params{K1: 1.2, B: 0.75}
}

func CustomParams(K1 float64, B float64) Params {
	return Params{K1: K1, B: B}
}

// IDF (Inverse Document Frequency) 逆向文件頻率
//
// 核心目標：計算「這個關鍵字的稀有度與資訊價值」。越罕見的詞，分數越高。
// 這裡使用的是 Elasticsearch 的 BM25 變體公式：
// IDF = ln(1 + (N - df + 0.5) / (df + 0.5))
//
// 變數解析：
//   - N: 總文件數 (Total Documents)
//   - df: 包含該關鍵字的文件數 (Document Frequency)
//
// 公式設計巧思：
//  1. 核心比例 (N - df) / df：詞彙越稀有 (df 越小)，比值越大，得分越高。
//  2. 平滑處理 (+ 0.5)：加在分子與分母，防止 df = 0 時發生除以零 (Divide-by-zero) 錯誤。
//  3. 取自然對數 (ln)：抑制極端罕見詞彙導致的分數暴走，讓分數平滑增長，避免蓋過 TF。
//  4. 加上 1 (+ 1)：確保即使詞彙極度常見 (例如 df > N/2)，算出的數值仍會 >= 0，
//     絕對不會因為包含常見詞彙而產生負分
func IDF(N, df int) float64 {
	return math.Log(1 + (float64(N)-float64(df)+0.5)/(float64(df)+0.5))
}

// TF (Term Frequency) 詞頻與長度懲罰
//
// 核心目標：計算關鍵字在文章中出現的次數，並解決「次數飽和」與「文章長度不公平」兩個問題。
//
// 公式：
// TF = (freq * (K1 + 1)) / (freq + K1 * (1 - B + B * docLen/avgDocLen))
//
// 變數解析：
//   - freq: 關鍵字在這篇特定文章裡「出現的次數」。
//   - docLen: 「這篇文章的長度」。
//   - avgDocLen: 整個資料庫裡「所有文章的平均長度」。
//   - p.K1 (預設 1.2): 飽和度參數。
//   - p.B  (預設 0.75): 長度懲罰參數。
//
// 設計巧思一：飽和度機制 (由 K1 控制)
// 當 freq 不斷增加時，TF 分數不會無限飆高，而是會受制於分母的 freq，
// 最終極限值會逼近 (K1 + 1)。這完美避免了「單純靠無腦塞關鍵字來衝高分數」的作弊行為。
//
// 設計巧思二：長度懲罰機制 (由 B 與 docLen/avgDocLen 控制)
// 公式右下角的因子： 1 - B + B * (docLen / avgDocLen)
//   - 若 docLen == avgDocLen：因子等於 1。完全沒有懲罰。
//   - 若 docLen >  avgDocLen (長文)：因子 > 1。導致 TF 分母變大，總分被壓低 (懲罰機制)。
//     藉此抵消長文章本來就比較容易命中關鍵字的優勢。
//   - 若 docLen <  avgDocLen (短文)：因子 < 1。導致 TF 分母變小，總分被放大 (獎勵機制)。
func TF(freq float64, docLen int, avgDocLen float64, params Params) float64 {

	// 計算飽和度分子
	numerator := freq * (params.K1 + 1.0)

	// 計算長度懲罰因子
	lengthPenalty := 1.0 - params.B + (params.B * (float64(docLen) / avgDocLen))

	// 計算包含飽和度與懲罰的分母
	denominator := freq + (params.K1 * lengthPenalty)

	return numerator / denominator
}

// Score 計算一個詞彙在文件中的最終 BM25 分數。
//
// 邏輯：IDF (單價/權重) * TF (數量/強度)
// 使用乘法而非加法，是因為乘法具有「消音器與放大器」的特性。
// 若文章未提及關鍵字 (TF=0)，則總分為 0，避免因為關鍵字很罕見 (高 IDF) 卻無中生有獲得高分。
func Score(freq float64, docLen int, avgDocLen float64, N int, df int, params Params, boost float64) float64 {
	return IDF(N, df) * TF(freq, docLen, avgDocLen, params) * boost
}
