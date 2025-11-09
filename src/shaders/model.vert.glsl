#if __VERSION__ >= 450
#define COMPAT_TEXTURE texture
layout(binding = 0) uniform UniformBufferObject0 {
	mat4 view, projection;
	mat4 lightMatrices[4];
	layout(offset = 688) vec3 cameraPosition;
};

layout(binding = 2) uniform UniformBufferObject2 {
	mat4 model,normalMatrix;
	int numJoints,numTargets,morphTargetTextureDimension,numVertices;
	vec4 morphTargetWeight[2];
	vec4 morphTargetOffset;
	float meshOutline;
};

layout(binding = 3) uniform sampler2D jointMatrices;
layout(binding = 4) uniform sampler2D morphTargetValues;

layout (constant_id = 0) const bool useJoint0 = false;
layout (constant_id = 1) const bool useJoint1 = false;
layout (constant_id = 2) const bool useNormal = false;
layout (constant_id = 3) const bool useTangent = false;
layout (constant_id = 4) const bool useVertColor = false;
layout (constant_id = 5) const bool useOutlineAttribute = false;

layout(location = 0) in int vertexId;
layout(location = 1) in vec3 position;
layout(location = 2) in vec2 uv;
layout(location = 3) in vec3 normalIn;
layout(location = 4) in vec4 tangentIn;
layout(location = 5) in vec4 vertColor;
layout(location = 6) in vec4 joints_0;
layout(location = 7) in vec4 weights_0;
layout(location = 8) in vec4 joints_1;
layout(location = 9) in vec4 weights_1;
layout(location = 10) in vec4 outlineAttributeIn;
layout(location = 0) out vec3 normal;
layout(location = 1) out vec3 tangent;
layout(location = 2) out vec3 bitangent;
layout(location = 3) out vec2 texcoord;
layout(location = 4) out vec4 vColor;
layout(location = 5) out vec3 worldSpacePos;
layout(location = 6) out vec4 lightSpacePos[4];
#else
#if __VERSION__ >= 130
#define COMPAT_VARYING out
#define COMPAT_ATTRIBUTE in
#define COMPAT_TEXTURE texture
#else
#extension GL_EXT_gpu_shader4 : enable
#define COMPAT_VARYING varying 
#define COMPAT_ATTRIBUTE attribute 
#define COMPAT_TEXTURE texture2D
#endif
uniform mat4 model, view, projection;
uniform mat4 normalMatrix;
uniform mat4 lightMatrices[4];
uniform sampler2D jointMatrices;
//uniform highp sampler2D morphTargetValues;
uniform sampler2D morphTargetValues;
uniform int numJoints;
uniform int numTargets;
uniform int morphTargetTextureDimension;
uniform vec4 morphTargetWeight[2];
uniform vec4 morphTargetOffset;
uniform int numVertices;
uniform float meshOutline;
uniform vec3 cameraPosition;
//gl_VertexID is not available in 1.2
COMPAT_ATTRIBUTE float vertexId;
COMPAT_ATTRIBUTE vec3 position;
COMPAT_ATTRIBUTE vec3 normalIn;
COMPAT_ATTRIBUTE vec4 tangentIn;
COMPAT_ATTRIBUTE vec2 uv;
COMPAT_ATTRIBUTE vec4 vertColor;
COMPAT_ATTRIBUTE vec4 joints_0;
COMPAT_ATTRIBUTE vec4 joints_1;
COMPAT_ATTRIBUTE vec4 weights_0;
COMPAT_ATTRIBUTE vec4 weights_1;
COMPAT_ATTRIBUTE vec4 outlineAttributeIn;
COMPAT_VARYING vec3 normal;
COMPAT_VARYING vec3 tangent;
COMPAT_VARYING vec3 bitangent;
COMPAT_VARYING vec2 texcoord;
COMPAT_VARYING vec4 vColor;
COMPAT_VARYING vec3 worldSpacePos;
COMPAT_VARYING vec4 lightSpacePos[4];


#define useJoint0 (weights_0.x+weights_0.y+weights_0.z+weights_0.w+weights_1.x+weights_1.y+weights_1.z+weights_1.w>0.0)
const bool useJoint1 = true;
const bool useNormal = true;
const bool useTangent = true;
const bool useVertColor = true;
const bool useOutlineAttribute = true;
#endif


mat4 getMatrixFromTexture(float index){
	mat4 mat;
#if __VERSION__ >= 450
	mat[0] = COMPAT_TEXTURE(jointMatrices,vec2(0.5/6.0,(index+0.5) / float(numJoints)));
	mat[1] = COMPAT_TEXTURE(jointMatrices,vec2(1.5/6.0,(index+0.5) / float(numJoints)));
	mat[2] = COMPAT_TEXTURE(jointMatrices,vec2(2.5/6.0,(index+0.5) / float(numJoints)));
#else
	// For older GLSL versions, use constant expressions where possible
	float yCoord = (index + 0.5) / float(numJoints);
	mat[0] = COMPAT_TEXTURE(jointMatrices,vec2(0.08333333, yCoord)); // 0.5/6.0 = 0.08333333
	mat[1] = COMPAT_TEXTURE(jointMatrices,vec2(0.25, yCoord));       // 1.5/6.0 = 0.25
	mat[2] = COMPAT_TEXTURE(jointMatrices,vec2(0.41666666, yCoord)); // 2.5/6.0 = 0.41666666
#endif
	mat[3] = vec4(0,0,0,1);
	return transpose(mat);
}

mat4 getNormalMatrixFromTexture(float index){
	mat4 mat;
#if __VERSION__ >= 450
	mat[0] = COMPAT_TEXTURE(jointMatrices,vec2(3.5/6.0,(index+0.5)/ float(numJoints)));
	mat[1] = COMPAT_TEXTURE(jointMatrices,vec2(4.5/6.0,(index+0.5)/ float(numJoints)));
	mat[2] = COMPAT_TEXTURE(jointMatrices,vec2(5.5/6.0,(index+0.5)/ float(numJoints)));
#else
	// For older GLSL versions, use constant expressions where possible
	float yCoord = (index + 0.5) / float(numJoints);
	mat[0] = COMPAT_TEXTURE(jointMatrices,vec2(0.58333333, yCoord)); // 3.5/6.0 = 0.58333333
	mat[1] = COMPAT_TEXTURE(jointMatrices,vec2(0.75, yCoord));       // 4.5/6.0 = 0.75
	mat[2] = COMPAT_TEXTURE(jointMatrices,vec2(0.91666666, yCoord)); // 5.5/6.0 = 0.91666666
#endif
	mat[3] = vec4(0,0,0,1);
	return transpose(mat);
}

mat4 getJointMatrix(){
	mat4 ret = mat4(0.0);
	ret += weights_0.x*getMatrixFromTexture(joints_0.x);
	ret += weights_0.y*getMatrixFromTexture(joints_0.y);
	ret += weights_0.z*getMatrixFromTexture(joints_0.z);
	ret += weights_0.w*getMatrixFromTexture(joints_0.w);
	if(useJoint1){
		ret += weights_1.x*getMatrixFromTexture(joints_1.x);
		ret += weights_1.y*getMatrixFromTexture(joints_1.y);
		ret += weights_1.z*getMatrixFromTexture(joints_1.z);
		ret += weights_1.w*getMatrixFromTexture(joints_1.w);
	}
	if(ret == mat4(0.0)){
		return mat4(1.0);
	}
	return ret;
}

mat3 getJointNormalMatrix(){
	mat4 ret = mat4(0.0);
	vec4 w1 = useJoint1?weights_1:vec4(0.0);
	ret += weights_0.x*getNormalMatrixFromTexture(joints_0.x);
	ret += weights_0.y*getNormalMatrixFromTexture(joints_0.y);
	ret += weights_0.z*getNormalMatrixFromTexture(joints_0.z);
	ret += weights_0.w*getNormalMatrixFromTexture(joints_0.w);
	ret += w1.x*getNormalMatrixFromTexture(joints_1.x);
	ret += w1.y*getNormalMatrixFromTexture(joints_1.y);
	ret += w1.z*getNormalMatrixFromTexture(joints_1.z);
	ret += w1.w*getNormalMatrixFromTexture(joints_1.w);
	if(ret == mat4(0.0)){
		return mat3(1.0);
	}
	return mat3(ret);
}

void main(void) {
	texcoord = uv;
	vColor = useVertColor?vertColor:vec4(1.0,1.0,1.0,1.0);
	vec4 pos = vec4(position, 1.0);
	normal = useNormal?normalIn:vec3(0.0,0.0,0.0);
	tangent = useTangent?vec3(tangentIn):vec3(0.0,0.0,0.0);
	vec4 outlineAttribute = useOutlineAttribute?outlineAttributeIn:vec4(0.0);
	
	if(morphTargetWeight[0][0] != 0.0){
		for(int idx = 0; idx < numTargets; ++idx)
		{
			float i = float(idx*numVertices+int(vertexId));
			vec2 xy = vec2((i+0.5)/float(morphTargetTextureDimension)-floor(i/float(morphTargetTextureDimension)),(floor(i/float(morphTargetTextureDimension))+0.5)/float(morphTargetTextureDimension));
			if(idx < int(morphTargetOffset[0])){
				pos += morphTargetWeight[idx/4][idx%4] * COMPAT_TEXTURE(morphTargetValues,xy);
			}else if(idx < int(morphTargetOffset[1])){
				normal += morphTargetWeight[idx/4][idx%4] * vec3(COMPAT_TEXTURE(morphTargetValues,xy));
			}else if(idx < int(morphTargetOffset[2])){
				tangent += morphTargetWeight[idx/4][idx%4] * vec3(COMPAT_TEXTURE(morphTargetValues,xy));
			}else if(idx < int(morphTargetOffset[3])){
				texcoord += morphTargetWeight[idx/4][idx%4] * vec2(COMPAT_TEXTURE(morphTargetValues,xy));
			}else{
				vColor += morphTargetWeight[idx/4][idx%4] * COMPAT_TEXTURE(morphTargetValues,xy);
			}
		}
	}
	
	if(useJoint0){
		mat4 jointMatrix = getJointMatrix();
		mat3 jointNormalMatrix = getJointNormalMatrix();
		normal = mat3(normalMatrix) * jointNormalMatrix * normal;
		vec4 tmp2 = model * jointMatrix * pos;
		
		if(outlineAttribute.w > 0.0){
			vec3 p = normalize(mat3(normalMatrix) * outlineAttribute.xyz)*outlineAttribute.w*meshOutline*length(cameraPosition-tmp2.xyz);
			tmp2.xyz += p;
		}else{
			vec3 p = normal*meshOutline*length(cameraPosition-tmp2.xyz);
			tmp2.xyz += p;
		}

		gl_Position = projection * view * tmp2;
		worldSpacePos = vec3(tmp2);
		for(int i = 0;i < 4;i++){
			lightSpacePos[i] = lightMatrices[i] * tmp2;
		}
	}else{
		if(normal.x+normal.y+normal.z != 0.0){
			normal = normalize(mat3(normalMatrix) * normal);
		}
		if(tangent.x+tangent.y+tangent.z != 0.0){
			tangent = normalize(vec3(model * vec4(tangent,0.0)));
			bitangent = cross(normal, tangent) * (useTangent?tangentIn.w:0.0);
		}
		vec4 tmp2 = model * pos;
		if(outlineAttribute.w > 0.0){
			vec3 p = normalize(mat3(normalMatrix) * outlineAttribute.xyz)*outlineAttribute.w*meshOutline*length(cameraPosition-tmp2.xyz);
			tmp2.xyz += p;
		}else{
			vec3 p = normal*meshOutline*length(cameraPosition-tmp2.xyz);
			tmp2.xyz += p;
		}

		gl_Position = projection * view * tmp2;
		worldSpacePos = vec3(tmp2);
		for(int i = 0;i < 4;i++){
			lightSpacePos[i] = lightMatrices[i] * tmp2;
		}
	}
	#if __VERSION__ >= 450
	gl_Position.y = -gl_Position.y;
	#endif
}
