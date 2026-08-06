cbuffer FontUniforms : register(b0) {
	float4 textColor;
	float4 resolution;
	float4 palAddGray;
	float4 palMulHue;
	float4 palNeg;
};

Texture2D tex : register(t0);
SamplerState s0 : register(s0);

struct VSOut {
	float4 pos : SV_Position;
	float2 fragTexCoord : TEXCOORD0;
};

float3 rgb2hsv(float3 c) {
	float4 K = float4(0.0, -1.0 / 3.0, 2.0 / 3.0, -1.0);
	float4 p = lerp(float4(c.bg, K.wz), float4(c.gb, K.xy), step(c.b, c.g));
	float4 q = lerp(float4(p.xyw, c.r), float4(c.r, p.yzx), step(p.x, c.r));
	float d = q.x - min(q.w, q.y);
	float e = 1.0e-10;
	return float3(abs(q.z + (q.w - q.y) / (6.0 * d + e)), d / (q.x + e), q.x);
}

float3 hsv2rgb(float3 c) {
	float4 K = float4(1.0, 2.0 / 3.0, 1.0 / 3.0, 3.0);
	float3 p = abs(frac(c.xxx + K.xyz) * 6.0 - K.www);
	return c.z * lerp(K.xxx, saturate(p - K.xxx), c.y);
}

float3 hue_shift(float3 color, float dhue) {
	float3 colorhsv = rgb2hsv(color);
	colorhsv.x = fmod(colorhsv.x + dhue, 1.0);
	return hsv2rgb(colorhsv);
}

float4 main(VSOut input) : SV_Target {
	float4 texColor = tex.Sample(s0, input.fragTexCoord);
	float4 sampled = float4(1.0, 1.0, 1.0, texColor.r);
	float4 c = min(textColor, float4(1.0, 1.0, 1.0, 1.0)) * sampled;

	float3 addV = palAddGray.xyz;
	float grayV = palAddGray.w;
	float3 mulV = palMulHue.xyz;
	float hueV = palMulHue.w;
	float negV = palNeg.x;

	if (hueV != 0.0) {
		c.rgb = hue_shift(c.rgb, hueV);
	}
	if (negV > 0.5) {
		c.rgb = c.aaa - c.rgb;
	}
	float grayL = (c.r + c.g + c.b) / 3.0;
	c.rgb = lerp(float3(grayL, grayL, grayL), c.rgb, 1.0 - grayV);
	c.rgb += addV * c.a;
	c.rgb *= mulV;

	return c;
}
