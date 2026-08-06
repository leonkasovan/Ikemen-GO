cbuffer PostUniforms : register(b0) {
	float4 TextureSize;
	float4 CurrentTime;
};

struct VSIn {
	float2 VertCoord : POSITION;
};

struct VSOut {
	float4 pos : SV_Position;
	float2 texcoord : TEXCOORD0;
};

VSOut main(VSIn input) {
	VSOut o;
	o.pos = float4(input.VertCoord, 0.0, 1.0);
	// D3D11 texture origin is top-left (GL is bottom-left): flip V so the
	// bottom of the viewport samples the bottom of the source texture.
	o.texcoord = float2((input.VertCoord.x + 1.0) * 0.5, (1.0 - input.VertCoord.y) * 0.5);
	return o;
}
