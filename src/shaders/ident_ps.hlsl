cbuffer PostUniforms : register(b0) {
	float4 TextureSize;
	float4 CurrentTime;
};

Texture2D Texture : register(t0);
SamplerState s0 : register(s0);

struct VSOut {
	float4 pos : SV_Position;
	float2 texcoord : TEXCOORD0;
};

float4 main(VSOut input) : SV_Target {
	return Texture.Sample(s0, input.texcoord);
}
