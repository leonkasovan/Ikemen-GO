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

struct VSIn {
	float2 position : POSITION;
	float2 uv : TEXCOORD;
};

struct VSOut {
	float4 pos : SV_Position;
	float2 uv : TEXCOORD0;
};

VSOut main(VSIn input) {
	VSOut output;
	output.uv = input.uv;
	output.pos = mul(projection, mul(modelview, float4(input.position, 0.0, 1.0)));
	return output;
}
