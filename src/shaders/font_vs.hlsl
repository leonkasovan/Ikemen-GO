cbuffer FontUniforms : register(b0) {
	float4 textColor;
	float4 resolution;
	float4 palAddGray;
	float4 palMulHue;
	float4 palNeg;
};

struct VSIn {
	float2 vert : POSITION;
	float2 vertTexCoord : TEXCOORD;
};

struct VSOut {
	float4 pos : SV_Position;
	float2 fragTexCoord : TEXCOORD0;
};

VSOut main(VSIn input) {
	VSOut o;
	o.fragTexCoord = input.vertTexCoord;

	float2 res = resolution.xy;
	if (res.x < 1.0) res.x = 1.0;
	if (res.y < 1.0) res.y = 1.0;

	float2 zeroToOne = input.vert / res;
	float2 zeroToTwo = zeroToOne * 2.0;
	float2 clipSpace = zeroToTwo - 1.0;

	o.pos = float4(clipSpace * float2(1.0, -1.0), 0.0, 1.0);
	return o;
}
