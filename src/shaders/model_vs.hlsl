// model_vs.hlsl — D3D11 port of model.vert.glsl (Ikemen-GO model pipeline, Phase A+B)
//
// Conventions vs the GLSL original:
//  * The engine's per-attribute flags (useJoint0/1, useNormal, ...) arrive via the
//    meshData/flagsData cbuffer fields instead of VK specialization constants or
//    GL data-driven #defines. SetModelPipeline writes them per primitive.
//  * The engine writes a 4-byte little-endian uint32 vertexId prefix block at the
//    start of each primitive's data. The input layout exposes it as input slot 0
//    (R32_UINT, semantic VERTEXID) so the morph-target math sees the sequential
//    per-vertex index; SV_VertexID cannot be used here because for indexed draws
//    it returns the index-buffer value, not the sequential vertex number.
//  * D3D11 NDC z is [0,1] (GL/VK-style matrices produce [-1,1]), so the final
//    clip position is remapped: pos.z = pos.z*0.5 + pos.w*0.5. NDC y matches GL
//    (+1 = top), so no Y flip is needed.
//  * Model matrices are column-major (mgl.Mat4), matching HLSL float4x4 default.

cbuffer ModelVS : register(b0) {
	float4x4 model;
	float4x4 normalMatrix;
	float4x4 view;
	float4x4 projection;
	float4x4 lightMatrices[4];
	float4 cameraPosition;
	int4 numData;                 // x=numJoints y=numTargets z=morphTargetTextureDimension w=numVertices
	float4 morphTargetWeight[2];
	float4 morphTargetOffset;
	float4 meshData;              // x=meshOutline y=useJoint0 z=useJoint1 w=useNormal
	float4 flagsData;             // x=useTangent y=useVertColor z=useOutlineAttribute w=pad
};

Texture2D<float4> jointMatrices : register(t0);
Texture2D<float4> morphTargetValues : register(t1);
SamplerState jointSampler : register(s0);
SamplerState morphSampler : register(s1);

struct VSIn {
	float3 position : POSITION;
	float2 uv : TEXCOORD;
	float3 normalIn : NORMAL;
	float4 tangentIn : TANGENT;
	float4 vertColor : COLOR;
	float4 joints_0 : JOINTS0;
	float4 weights_0 : WEIGHTS0;
	float4 joints_1 : JOINTS1;
	float4 weights_1 : WEIGHTS1;
	float4 outlineAttributeIn : OUTLINE;
	uint vid : VERTEXID;
};

struct VSOut {
	float4 pos : SV_Position;
	float3 normal : TEXCOORD1;
	float3 tangent : TEXCOORD2;
	float3 bitangent : TEXCOORD3;
	float2 texcoord : TEXCOORD0;
	float4 vColor : TEXCOORD4;
	float3 worldSpacePos : TEXCOORD5;
	float4 lightSpacePos0 : TEXCOORD6;
	float4 lightSpacePos1 : TEXCOORD7;
	float4 lightSpacePos2 : TEXCOORD8;
	float4 lightSpacePos3 : TEXCOORD9;
};

float4x4 getMatrixFromTexture(float index) {
	float4x4 mat;
	mat[0] = jointMatrices.SampleLevel(jointSampler, float2(0.5 / 6.0, (index + 0.5) / (float)numData.x), 0);
	mat[1] = jointMatrices.SampleLevel(jointSampler, float2(1.5 / 6.0, (index + 0.5) / (float)numData.x), 0);
	mat[2] = jointMatrices.SampleLevel(jointSampler, float2(2.5 / 6.0, (index + 0.5) / (float)numData.x), 0);
	mat[3] = float4(0.0, 0.0, 0.0, 1.0);
	return transpose(mat);
}

float4x4 getNormalMatrixFromTexture(float index) {
	float4x4 mat;
	mat[0] = jointMatrices.SampleLevel(jointSampler, float2(3.5 / 6.0, (index + 0.5) / (float)numData.x), 0);
	mat[1] = jointMatrices.SampleLevel(jointSampler, float2(4.5 / 6.0, (index + 0.5) / (float)numData.x), 0);
	mat[2] = jointMatrices.SampleLevel(jointSampler, float2(5.5 / 6.0, (index + 0.5) / (float)numData.x), 0);
	mat[3] = float4(0.0, 0.0, 0.0, 1.0);
	return transpose(mat);
}

bool isZeroMatrix(float4x4 m) {
	return all(m[0] == 0) && all(m[1] == 0) && all(m[2] == 0) && all(m[3] == 0);
}

float4x4 getJointMatrix(VSIn input) {
	float4x4 ret = (float4x4)0;
	ret += input.weights_0.x * getMatrixFromTexture(input.joints_0.x);
	ret += input.weights_0.y * getMatrixFromTexture(input.joints_0.y);
	ret += input.weights_0.z * getMatrixFromTexture(input.joints_0.z);
	ret += input.weights_0.w * getMatrixFromTexture(input.joints_0.w);
	if (meshData.z != 0.0) {
		ret += input.weights_1.x * getMatrixFromTexture(input.joints_1.x);
		ret += input.weights_1.y * getMatrixFromTexture(input.joints_1.y);
		ret += input.weights_1.z * getMatrixFromTexture(input.joints_1.z);
		ret += input.weights_1.w * getMatrixFromTexture(input.joints_1.w);
	}
	if (isZeroMatrix(ret)) {
		return (float4x4)1;
	}
	return ret;
}

float3x3 getJointNormalMatrix(VSIn input) {
	float4x4 ret = (float4x4)0;
	float4 w1 = (meshData.z != 0.0) ? input.weights_1 : float4(0, 0, 0, 0);
	ret += input.weights_0.x * getNormalMatrixFromTexture(input.joints_0.x);
	ret += input.weights_0.y * getNormalMatrixFromTexture(input.joints_0.y);
	ret += input.weights_0.z * getNormalMatrixFromTexture(input.joints_0.z);
	ret += input.weights_0.w * getNormalMatrixFromTexture(input.joints_0.w);
	ret += w1.x * getNormalMatrixFromTexture(input.joints_1.x);
	ret += w1.y * getNormalMatrixFromTexture(input.joints_1.y);
	ret += w1.z * getNormalMatrixFromTexture(input.joints_1.z);
	ret += w1.w * getNormalMatrixFromTexture(input.joints_1.w);
	if (isZeroMatrix(ret)) {
		return (float3x3)1;
	}
	return (float3x3)ret;
}

VSOut main(VSIn input) {
	VSOut output;
	output.normal = float3(0, 0, 0);
	output.tangent = float3(0, 0, 0);
	output.bitangent = float3(0, 0, 0);
	output.worldSpacePos = float3(0, 0, 0);
	output.texcoord = float2(0, 0);
	output.vColor = float4(1, 1, 1, 1);
	output.lightSpacePos0 = float4(0, 0, 0, 0);
	output.lightSpacePos1 = float4(0, 0, 0, 0);
	output.lightSpacePos2 = float4(0, 0, 0, 0);
	output.lightSpacePos3 = float4(0, 0, 0, 0);

	output.texcoord = input.uv;
	output.vColor = (flagsData.y != 0.0) ? input.vertColor : float4(1, 1, 1, 1);
	float4 pos = float4(input.position, 1.0);
	output.normal = (meshData.w != 0.0) ? input.normalIn : float3(0, 0, 0);
	output.tangent = (flagsData.x != 0.0) ? input.tangentIn.xyz : float3(0, 0, 0);
	float4 outlineAttribute = (flagsData.z != 0.0) ? input.outlineAttributeIn : float4(0, 0, 0, 0);

	if (morphTargetWeight[0].x != 0.0) {
		for (int idx = 0; idx < numData.y; ++idx) {
			float fIdx = (float)idx;
			float i = fIdx * (float)numData.w + (float)input.vid;
			float2 xy = float2(
				(i + 0.5) / (float)numData.z - floor(i / (float)numData.z),
				(floor(i / (float)numData.z) + 0.5) / (float)numData.z);
			float4 w = (idx < 4) ? morphTargetWeight[0] : morphTargetWeight[1];
			int m = idx - (idx / 4) * 4;
			float weight = (m == 0) ? w.x : (m == 1) ? w.y : (m == 2) ? w.z : w.w;
			float4 mSample = morphTargetValues.SampleLevel(morphSampler, xy, 0);
			if (fIdx < morphTargetOffset.x) pos += weight * mSample;
			else if (fIdx < morphTargetOffset.y) output.normal += weight * mSample.xyz;
			else if (fIdx < morphTargetOffset.z) output.tangent += weight * mSample.xyz;
			else if (fIdx < morphTargetOffset.w) output.texcoord += weight * mSample.xy;
			else output.vColor += weight * mSample;
		}
	}

	float4 tmp2;
	if (meshData.y != 0.0) {
		float4x4 jointMatrix = getJointMatrix(input);
		float3x3 jointNormalMatrix = getJointNormalMatrix(input);
		output.normal = mul((float3x3)normalMatrix, mul(jointNormalMatrix, output.normal));
		tmp2 = mul(model, mul(jointMatrix, pos));
		if (outlineAttribute.w > 0.0) {
			float3 p = normalize(mul((float3x3)normalMatrix, outlineAttribute.xyz)) * outlineAttribute.w * meshData.x * length(cameraPosition.xyz - tmp2.xyz);
			tmp2.xyz += p;
		} else {
			float3 p = output.normal * meshData.x * length(cameraPosition.xyz - tmp2.xyz);
			tmp2.xyz += p;
		}
	} else {
		if (output.normal.x + output.normal.y + output.normal.z != 0.0) {
			output.normal = normalize(mul((float3x3)normalMatrix, output.normal));
		}
		if (output.tangent.x + output.tangent.y + output.tangent.z != 0.0) {
			output.tangent = normalize(mul((float3x3)model, output.tangent));
			output.bitangent = cross(output.normal, output.tangent) * ((flagsData.x != 0.0) ? input.tangentIn.w : 0.0);
		}
		tmp2 = mul(model, pos);
		if (outlineAttribute.w > 0.0) {
			float3 p = normalize(mul((float3x3)normalMatrix, outlineAttribute.xyz)) * outlineAttribute.w * meshData.x * length(cameraPosition.xyz - tmp2.xyz);
			tmp2.xyz += p;
		} else {
			float3 p = output.normal * meshData.x * length(cameraPosition.xyz - tmp2.xyz);
			tmp2.xyz += p;
		}
	}

	output.pos = mul(projection, mul(view, tmp2));
	output.worldSpacePos = tmp2.xyz;
	output.lightSpacePos0 = mul(lightMatrices[0], tmp2);
	output.lightSpacePos1 = mul(lightMatrices[1], tmp2);
	output.lightSpacePos2 = mul(lightMatrices[2], tmp2);
	output.lightSpacePos3 = mul(lightMatrices[3], tmp2);

	// GL-style matrices output NDC z in [-1,1]; D3D11 requires [0,1].
	output.pos.z = output.pos.z * 0.5 + output.pos.w * 0.5;
	return output;
}
