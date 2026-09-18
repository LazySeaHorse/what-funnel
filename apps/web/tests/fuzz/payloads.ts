/**
 * Curated fuzz dictionary for UI and form inputs.
 * Covers buffer boundaries, unicode, RTL, injection patterns, format strings, and numbers.
 */

export const FUZZ_PAYLOADS: string[] = [
	// Empty and whitespace
	'',
	' ',
	'   \t   \n   \r\n   ',
	'\u200B\u200C\u200D\uFEFF', // Zero-width spaces & joiners

	// Length boundaries
	'A'.repeat(255),
	'B'.repeat(1024),
	'C'.repeat(5000),

	// Special Unicode & Emojis
	'🎉🚀🔥💎⚡️✨🤖👾',
	'👩‍👩‍👧‍👦 👨‍👩‍👧‍👦 🏳️‍🌈 🏴‍☠️', // Surrogate pairs & compound emojis
	'T̶̢̛h̷̢e̷̢ ̴V̴o̸i̸d̴ ̷C̷o̷n̷s̷u̷m̷e̷s̶', // Zalgo text
	'ñ, é, ö, ü, ß, å, ø, ç', // Diacritics
	'日本語のテスト文字列', // Japanese Kanji/Kana
	'한글 테스트 문자열', // Korean Hangul
	'مرحبا بالعالم - هذا نص للاختبار', // Arabic (RTL)
	'שלום עולם - טקסט בדיקה', // Hebrew (RTL)

	// Injection & Scripting Payloads (XSS, Template, SQL)
	'<script>alert("fuzz-xss")</script>',
	'<img src=x onerror=alert(1)>',
	'<svg onload=alert(document.domain)>',
	'"><script>alert(1)</script>',
	'javascript:alert(1)',
	'{{7*7}}',
	'${7*7}',
	"#{7*7}",
	'<%= 7*7 %>',
	"{{constructor.constructor('return this')()}}",
	"' OR '1'='1",
	"'; DROP TABLE accounts; --",
	'admin\'--',
	'" OR ""="',

	// Format strings & path traversal
	'%s%s%s%s%s%s%s%s%s%s',
	'%d%d%d%d%d%d',
	'%x%x%x%x',
	'../../../../../../etc/passwd',
	'..\\..\\..\\..\\windows\\win.ini',
	'file:///etc/passwd',

	// Numbers & numeric edge cases
	'0',
	'-1',
	'-9999999999999999',
	'9999999999999999999999999999999999999999',
	'1e308',
	'-1e308',
	'NaN',
	'Infinity',
	'-Infinity',
	'0.00000000000000000001',
	'3.14159265358979323846',

	// JSON & structural strings
	'{"key": "value", "nested": [1, 2, 3]}',
	'{"__proto__": {"polluted": true}}',
	'[null, undefined, true, false, 0]',
	'undefined',
	'null',
	'[object Object]',

	// Punctuation and control characters
	'!@#$%^&*()_+~`|}{[]:;?><,./-=',
	'\x00\x01\x02\x03\x04\x05\x06\x07\x08',
	'\x1b[31mRed Text\x1b[0m'
];
