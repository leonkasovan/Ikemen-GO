cbuffer SpriteUniforms : register(b0) {
	float4x4 modelview;
	float4x4 projection;
	float4 x1x2x4x3;
	float4 tint;
	float4 palUV;
	float4 add;
	float4 mult;
	float4 alphaGrayHue;
	int4 maskFlatRgbaTrapez;
	float4 negTime;
	float4 iResolution;
	float4 p0;
	float4 p1;
	float4 p2;
	float4 p3;
	float4 p4;
	float4 p5;
	float4 p6;
	float4 p7;
	float4 p8;
	float4 p9;
	float4 p10;
	float4 p11;
	float4 p12;
	float4 p13;
	float4 p14;
	float4 p15;
};

Texture2D tex : register(t0);
Texture2D pal : register(t1);
SamplerState s0 : register(s0);
SamplerState s1 : register(s1);

struct VSOut {
	float4 pos : SV_Position;
	float2 uv : TEXCOORD0;
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
	float alpha = alphaGrayHue.x;
	float gray = alphaGrayHue.y;
	float hue = alphaGrayHue.z;
	int mask = maskFlatRgbaTrapez.x;
	int isFlat = maskFlatRgbaTrapez.y;
	int isRgba = maskFlatRgbaTrapez.z;
	int isTrapez = maskFlatRgbaTrapez.w;
	float neg = negTime.x;

	float4 c;
	float3 neg_base = float3(1.0, 1.0, 1.0);
	float3 final_add = add.xyz;
	float4 final_mul = float4(mult.xyz, alpha);

	if (isFlat != 0) {
		c = tint;
		neg_base *= c.a;
		final_add *= c.a;
		final_mul.rgb *= alpha;
	} else {
		float2 uv = input.uv;
		if (isTrapez != 0) {
			float2 bounds = lerp(x1x2x4x3.zw, x1x2x4x3.xy, uv.y);
			float gap = bounds[1] - bounds[0];
			uv.x = (input.pos.x - bounds[0]) / gap;
		}

		c = tex.Sample(s0, uv);

		if (isRgba != 0) {
			if (mask == -1) c.a = 1.0;
			neg_base *= c.a;
			final_add *= c.a;
			final_mul.rgb *= alpha;
		} else {
			c = pal.Sample(s1, float2(palUV.x + palUV.z * c.r * 0.9966, palUV.y));
			if (mask == -1) c.a = 1.0;
		}
	}

	if (hue != 0.0) {
		c.rgb = hue_shift(c.rgb, hue);
	}
	if (neg > 0.5) {
		c.rgb = neg_base - c.rgb;
	}
	float grayL = (c.r + c.g + c.b) / 3.0;
	c.rgb = lerp(float3(grayL, grayL, grayL), c.rgb, 1.0 - gray);
	c.rgb += final_add;
	c *= final_mul;

	if (isFlat == 0) {
		c.rgb = lerp(c.rgb, tint.rgb * c.a, tint.a);
	}

	return c;
}
