// model_ps.hlsl — D3D11 port of model.frag.glsl (Ikemen-GO model pipeline, Phase A+B)
//
// Conventions vs the GLSL original:
//  * All three GLSL UBOs (EnvironmentUniform, MaterialUniform, UniformBufferObject)
//    are merged into one cbuffer whose layout matches dxModelPSUniforms exactly.
//  * useXxx flags are cbuffer floats (0/1) instead of bool/specialization consts.
//  * Shadow maps are not implemented in Phase A+B: useShadowMap is always 0, so
//    the shadow functions early-return 1.0 and shadowCubeMap is never sampled.
//  * Texture slots: t0 tex, t1 normalMap, t2 metallicRoughnessMap, t3
//    ambientOcclusionMap, t4 emissionMap, t5 lambertianEnv (cube), t6 GGXEnv
//    (cube), t7 GGXLUT, t8 shadowCubeMap (cube-array, unused in A+B).

cbuffer ModelPS : register(b0) {
	float4 lights[4][4];            // 4 lights x 4 float4
	float4 environmentRotation[3];  // 3 columns (mat3)
	float4 cameraPosition;
	float4 envMisc;                 // x=environmentIntensity y=mipCount
	float4 texTransform[3];
	float4 normalMapTransform[3];
	float4 metallicRoughnessMapTransform[3];
	float4 ambientOcclusionMapTransform[3];
	float4 emissionMapTransform[3];
	float4 baseColorFactor;
	float4 emission;
	float4 metallicRoughness;
	float4 matMisc;                 // x=ambientOcclusionStrength y=alphaThreshold z=unlit w=enableAlpha
	float4 palfxMisc;               // x=gray y=hue z=meshOutline w=neg
	float4 add;
	float4 mult;
	float4 texFlags;                // x=useTexture y=useNormalMap z=useMetallicRoughnessMap w=useEmissionMap
	float4 miscFlags;               // x=useAmbientOcclusionMap y=useShadowMap
};

Texture2D tex : register(t0);
Texture2D normalMap : register(t1);
Texture2D metallicRoughnessMap : register(t2);
Texture2D ambientOcclusionMap : register(t3);
Texture2D emissionMap : register(t4);
TextureCube lambertianEnvSampler : register(t5);
TextureCube GGXEnvSampler : register(t6);
Texture2D GGXLUT : register(t7);
TextureCubeArray shadowCubeMap : register(t8);

SamplerState s0 : register(s0);
SamplerState s1 : register(s1);
SamplerState s2 : register(s2);
SamplerState s3 : register(s3);
SamplerState s4 : register(s4);
SamplerState s5 : register(s5);
SamplerState s6 : register(s6);
SamplerState s7 : register(s7);
SamplerState s8 : register(s8);

struct PSIn {
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
	bool frontFacing : SV_IsFrontFace;
};

static const float PI = 3.14159265358979;
static const int LightType_Directional = 0;
static const int LightType_Point = 1;
static const int LightType_Spot = 2;

float clampedDot(float3 x, float3 y) {
	return clamp(dot(x, y), 0.0, 1.0);
}

// mat3 (3 float4 columns) * vec3, column-major like mgl.Mat3 / GLSL mat3.
float3 mulMat3(float4 c0, float4 c1, float4 c2, float3 v) {
	return c0.xyz * v.x + c1.xyz * v.y + c2.xyz * v.z;
}

float DirectionalLightShadowCalculation(int index, float4 lightSpacePos, float NdotL, float shadowBias) {
	if (miscFlags.y == 0.0) {
		return 1.0;
	}
	// Perspective divide and [0,1] range.
	float3 projCoords = lightSpacePos.xyz / lightSpacePos.w;
	projCoords = projCoords * 0.5 + 0.5;
	float epsilon = 1.0 / 1024.0;
	float2 xy = float2(clamp(projCoords.x, epsilon, 1.0 - epsilon), clamp(projCoords.y, epsilon, 1.0 - epsilon));
	float closestDepth = shadowCubeMap.SampleLevel(s8, float4(1.0, -(xy.y * 2.0 - 1.0), -(xy.x * 2.0 - 1.0), (float)index), 0).r;
	float currentDepth = projCoords.z;
	float bias = shadowBias * tan(acos(NdotL));
	float shadow = closestDepth - currentDepth > -bias ? 1.0 : 0.0;
	return shadow;
}

float SpotLightShadowCalculation(int index, float3 pointToLight, float4 lightSpacePos, float NdotL, float farPlane, float shadowBias) {
	if (miscFlags.y == 0.0) {
		return 1.0;
	}
	float epsilon = 1.0 / 1024.0;
	float2 xy = float2(clamp(lightSpacePos.x, epsilon, 1.0 - epsilon), clamp(lightSpacePos.y, epsilon, 1.0 - epsilon));
	float closestDepth = shadowCubeMap.SampleLevel(s8, float4(1.0, -(xy.y * 2.0 - 1.0), -(xy.x * 2.0 - 1.0), (float)index), 0).r;
	closestDepth *= farPlane;
	float currentDepth = length(pointToLight);
	float bias = shadowBias * tan(acos(NdotL));
	float shadow = currentDepth - closestDepth < bias ? 1.0 : 0.0;
	return shadow;
}

float PointLightShadowCalculation(int index, float3 pointToLight, float NdotL, float farPlane, float shadowBias) {
	if (miscFlags.y == 0.0) {
		return 1.0;
	}
	float3 xyz = -pointToLight;
	float closestDepth = shadowCubeMap.SampleLevel(s8, float4(xyz, (float)index), 0).r;
	closestDepth *= farPlane;
	float currentDepth = length(pointToLight);
	float bias = shadowBias * tan(acos(NdotL));
	float shadow = currentDepth - closestDepth < bias ? 1.0 : 0.0;
	return shadow;
}

float3 getNormal(PSIn input) {
	float2 uv_dx = ddx(input.texcoord);
	float2 uv_dy = ddy(input.texcoord);
	if (length(uv_dx) <= 1e-2) {
		uv_dx = float2(1.0, 0.0);
	}
	if (length(uv_dy) <= 1e-2) {
		uv_dy = float2(0.0, 1.0);
	}
	float3 t_ = (uv_dy.y * ddx(input.worldSpacePos) - uv_dx.y * ddy(input.worldSpacePos)) /
		(uv_dx.x * uv_dy.y - uv_dy.x * uv_dx.y);
	float3 n, t, b, ng;
	if (input.normal.x + input.normal.y + input.normal.z != 0.0) {
		if (input.tangent.x + input.tangent.y + input.tangent.z != 0.0) {
			t = normalize(input.tangent);
			b = normalize(input.bitangent);
			ng = normalize(input.normal);
		} else {
			ng = normalize(input.normal);
			t = normalize(t_ - ng * dot(ng, t_));
			b = cross(ng, t);
		}
	} else {
		ng = normalize(cross(ddx(input.worldSpacePos), ddy(input.worldSpacePos)));
		t = normalize(t_ - ng * dot(ng, t_));
		b = cross(ng, t);
	}
	if (!input.frontFacing) {
		t *= -1.0;
		b *= -1.0;
		ng *= -1.0;
	}
	if (texFlags.y != 0.0) {
		float3 nMap = normalMap.Sample(s1, mulMat3(normalMapTransform[0], normalMapTransform[1], normalMapTransform[2], float3(input.texcoord, 1.0)).xy).xyz * 2.0 - float3(1.0, 1.0, 1.0);
		return normalize(mulMat3(float4(t, 0), float4(b, 0), float4(ng, 0), nMap));
	}
	return ng;
}

float getRangeAttenuation(float range, float distance) {
	if (range <= 0.0) {
		return 1.0 / pow(distance, 2.0);
	}
	return max(min(1.0 - pow(distance / range, 4.0), 1.0), 0.0) / pow(distance, 2.0);
}

float getSpotAttenuation(float3 pointToLight, float3 spotDirection, float outerConeCos, float innerConeCos) {
	float actualCos = dot(normalize(spotDirection), normalize(-pointToLight));
	if (actualCos > outerConeCos) {
		if (actualCos < innerConeCos) {
			float angularAttenuation = (actualCos - outerConeCos) / (innerConeCos - outerConeCos);
			return angularAttenuation * angularAttenuation;
		}
		return 1.0;
	}
	return 0.0;
}

float3 getLighIntensity(int lightType, float3 lightColor, float lightIntensity, float lightRange, float3 lightDirection, float outerConeCos, float innerConeCos, float3 pointToLight) {
	float rangeAttenuation = 1.0;
	float spotAttenuation = 1.0;
	if (lightType != LightType_Directional) {
		rangeAttenuation = getRangeAttenuation(lightRange, length(pointToLight));
	}
	if (lightType == LightType_Spot) {
		spotAttenuation = getSpotAttenuation(pointToLight, lightDirection, outerConeCos, innerConeCos);
	}
	return rangeAttenuation * spotAttenuation * lightIntensity * lightColor;
}

float3 F_Schlick(float3 f0, float3 f90, float VdotH) {
	return f0 + (f90 - f0) * pow(clamp(1.0 - VdotH, 0.0, 1.0), 5.0);
}

float V_GGX(float NdotL, float NdotV, float alphaRoughness) {
	float alphaRoughnessSq = alphaRoughness * alphaRoughness;
	float GGXV = NdotL * sqrt(NdotV * NdotV * (1.0 - alphaRoughnessSq) + alphaRoughnessSq);
	float GGXL = NdotV * sqrt(NdotL * NdotL * (1.0 - alphaRoughnessSq) + alphaRoughnessSq);
	float GGX = GGXV + GGXL;
	if (GGX > 0.0) {
		return 0.5 / GGX;
	}
	return 0.0;
}

float D_GGX(float NdotH, float alphaRoughness) {
	float alphaRoughnessSq = alphaRoughness * alphaRoughness;
	float f = (NdotH * NdotH) * (alphaRoughnessSq - 1.0) + 1.0;
	return alphaRoughnessSq / (PI * f * f);
}

float3 BRDF_lambertian(float3 f0, float3 f90, float3 diffuseColor, float specularWeight, float VdotH) {
	return (1.0 - specularWeight * F_Schlick(f0, f90, VdotH)) * (diffuseColor / PI);
}

float3 BRDF_specularGGX(float3 f0, float3 f90, float alphaRoughness, float specularWeight, float VdotH, float NdotL, float NdotV, float NdotH) {
	float3 F = F_Schlick(f0, f90, VdotH);
	float Vis = V_GGX(NdotL, NdotV, alphaRoughness);
	float D = D_GGX(NdotH, alphaRoughness);
	return specularWeight * F * Vis * D;
}

float3 getDiffuseLight(float3 n) {
	return lambertianEnvSampler.Sample(s5, mulMat3(environmentRotation[0], environmentRotation[1], environmentRotation[2], n)).rgb * envMisc.x;
}

float4 getSpecularSample(float3 reflection, float lod) {
	return GGXEnvSampler.SampleLevel(s6, mulMat3(environmentRotation[0], environmentRotation[1], environmentRotation[2], reflection), lod) * envMisc.x;
}

float3 getIBLGGXFresnel(float3 n, float3 v, float roughness, float3 F0, float specularWeight) {
	float NdotV = clampedDot(n, v);
	float2 brdfSamplePoint = clamp(float2(NdotV, roughness), float2(0.0, 0.0), float2(1.0, 1.0));
	float2 f_ab = GGXLUT.Sample(s7, brdfSamplePoint).rg;
	float3 Fr = max(float3(1.0 - roughness, 1.0 - roughness, 1.0 - roughness), F0) - F0;
	float3 k_S = F0 + Fr * pow(1.0 - NdotV, 5.0);
	float3 FssEss = specularWeight * (k_S * f_ab.x + f_ab.y);

	float Ems = (1.0 - (f_ab.x + f_ab.y));
	float3 F_avg = specularWeight * (F0 + (1.0 - F0) / 21.0);
	float3 FmsEms = Ems * FssEss * F_avg / (1.0 - F_avg * Ems);

	return FssEss + FmsEms;
}

float3 getIBLRadianceGGX(float3 n, float3 v, float roughness) {
	float NdotV = clampedDot(n, v);
	float lod = roughness * (float)((int)envMisc.y - 1);
	float3 reflection = normalize(reflect(-v, n));
	float4 specularSample = getSpecularSample(reflection, lod);
	return specularSample.rgb;
}

float3 ibl(float3 n, float3 v, float metallic, float roughness, float3 albedo) {
	float3 f_diffuse = getDiffuseLight(n) * albedo.rgb;
	float3 f_specular_metal = getIBLRadianceGGX(n, v, roughness);
	float3 f_specular_dielectric = f_specular_metal;
	float3 f_metal_fresnel_ibl = getIBLGGXFresnel(n, v, roughness, albedo.rgb, 1.0);
	float3 f_metal_brdf_ibl = f_metal_fresnel_ibl * f_specular_metal;
	float3 f_dielectric_fresnel_ibl = getIBLGGXFresnel(n, v, roughness, float3(0.04, 0.04, 0.04), 1.0);
	float3 f_dielectric_brdf_ibl = f_diffuse * (1.0 - f_dielectric_fresnel_ibl) + f_specular_dielectric * f_dielectric_fresnel_ibl;

	float3 color = f_dielectric_brdf_ibl * (1.0 - metallic) + f_metal_brdf_ibl * metallic;
	return color;
}

float4 lightSpacePosFor(int i, PSIn input) {
	if (i == 0) return input.lightSpacePos0;
	if (i == 1) return input.lightSpacePos1;
	if (i == 2) return input.lightSpacePos2;
	return input.lightSpacePos3;
}

float3 pbr(PSIn input, float3 nIn, float3 albedo, float metallic, float roughness, float ao) {
	float3 f0 = float3(0.04, 0.04, 0.04) + (albedo - float3(0.04, 0.04, 0.04)) * metallic;
	float3 f90 = float3(1.0, 1.0, 1.0);
	float specularWeight = 1.0;
	float3 f_specular = float3(0.0, 0.0, 0.0);
	float3 f_diffuse = float3(0.0, 0.0, 0.0);
	float3 c_diff = albedo * (1.0 - metallic);
	float3 v = normalize(cameraPosition.xyz - input.worldSpacePos);
	float3 n = normalize(nIn);

	for (int i = 0; i < 4; ++i) {
		if (lights[i][1].x + lights[i][1].y + lights[i][1].z > 0.0) {
			float3 pointToLight = float3(0.0, 0.0, 0.0);
			int lightType = (int)lights[i][3].y;
			if (lightType == LightType_Directional) {
				pointToLight = -lights[i][0].xyz;
			} else {
				pointToLight = lights[i][2].xyz - input.worldSpacePos;
			}
			float3 l = normalize(pointToLight);
			float3 h = normalize(l + v);
			float NdotL = clampedDot(n, l);
			float NdotV = clampedDot(n, v);
			float NdotH = clampedDot(n, h);
			float VdotH = clampedDot(v, h);
			if (NdotL > 0.0 || NdotV > 0.0) {
				float3 intensity = getLighIntensity(lightType, lights[i][1].xyz, lights[i][1].w, lights[i][0].w, lights[i][0].xyz, lights[i][3].x, lights[i][2].w, pointToLight);
				float3 l_diffuse = float3(0.0, 0.0, 0.0);
				float3 l_specular = float3(0.0, 0.0, 0.0);
				l_diffuse += intensity * NdotL * BRDF_lambertian(f0, f90, c_diff, specularWeight, VdotH);
				l_specular += intensity * NdotL * BRDF_specularGGX(f0, f90, roughness * roughness, specularWeight, VdotH, NdotL, NdotV, NdotH);
				float shadow = 1.0;
				if (lightType == LightType_Point) {
					shadow = PointLightShadowCalculation(i, pointToLight, NdotL, lights[i][3].w, lights[i][3].z);
				} else if (lightType == LightType_Directional) {
					shadow = DirectionalLightShadowCalculation(i, lightSpacePosFor(i, input), NdotL, lights[i][3].z);
				} else {
					shadow = SpotLightShadowCalculation(i, pointToLight, lightSpacePosFor(i, input), NdotL, lights[i][3].w, lights[i][3].z);
				}
				f_diffuse += l_diffuse * shadow;
				f_specular += l_specular * shadow;
			}
		}
	}
	float3 f_ibl = float3(0.0, 0.0, 0.0);
	if (envMisc.x > 0.0) {
		f_ibl = ibl(n, v, metallic, roughness, albedo);
	}
	float3 color = f_diffuse + f_specular + ao * f_ibl;
	return color;
}

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

float4 main(PSIn input) : SV_Target {
	float4 FragColor = float4(1.0, 1.0, 1.0, 1.0);
	if (texFlags.x != 0.0) {
		FragColor = tex.Sample(s0, mulMat3(texTransform[0], texTransform[1], texTransform[2], float3(input.texcoord, 1.0)));
		// convert texture to linear space
		FragColor.rgb = pow(FragColor.rgb, float3(2.2, 2.2, 2.2));
	}
	FragColor *= baseColorFactor;
	FragColor *= input.vColor;
	if (palfxMisc.z > 0.0) {
		FragColor.rgb = float3(0.0, 0.0, 0.0);
	} else if (matMisc.z == 0.0) {
		float3 normalF = input.normal;
		if (texFlags.y != 0.0) {
			normalF = getNormal(input);
		}
		float2 metallicRoughnessF = metallicRoughness.xy;
		if (texFlags.z != 0.0) {
			metallicRoughnessF = metallicRoughnessMap.Sample(s2, mulMat3(metallicRoughnessMapTransform[0], metallicRoughnessMapTransform[1], metallicRoughnessMapTransform[2], float3(input.texcoord, 1.0))).bg;
		}
		float ambientOcclusion = 1.0;
		if (matMisc.x > 0.0) {
			ambientOcclusion = 1.0 + matMisc.x * (ambientOcclusionMap.Sample(s3, mulMat3(ambientOcclusionMapTransform[0], ambientOcclusionMapTransform[1], ambientOcclusionMapTransform[2], float3(input.texcoord, 1.0))).r - 1.0);
		}
		// PBR returns color in linear space
		FragColor.rgb = pbr(input, normalF, FragColor.rgb, metallicRoughnessF.x, metallicRoughnessF.y, ambientOcclusion);

		// Emission is also added in linear space
		if (texFlags.w != 0.0) {
			FragColor.rgb += emission.rgb * pow(emissionMap.Sample(s4, mulMat3(emissionMapTransform[0], emissionMapTransform[1], emissionMapTransform[2], float3(input.texcoord, 1.0))).rgb, float3(2.2, 2.2, 2.2));
		} else {
			FragColor.rgb += emission.rgb;
		}
	}

	// PBR output (FragColor.rgb) is now LINEAR, pre-multiplied by FragColor.a here
	FragColor.rgb *= FragColor.a;

	if (matMisc.w == 0.0) {
		if (FragColor.a < matMisc.y) {
			discard;
		} else {
			FragColor.a = 1.0;
		}
	} else if (FragColor.a <= 0.0) {
		discard;
	}

	// Gamma Correction before all PalFX operations
	float3 c_straight = pow(FragColor.rgb, float3(1.0 / 2.2, 1.0 / 2.2, 1.0 / 2.2));
	float alpha = FragColor.a;

	// Un-premultiply to get straight-alpha color
	if (alpha > 0.0) {
		c_straight /= alpha;
		c_straight = clamp(c_straight, 0.0, 1.0);
	}

	// Apply PalFX
	if (palfxMisc.y != 0.0) {
		c_straight = hue_shift(c_straight, palfxMisc.y);
	}

	// INVERSION FIX: Correctly applied on un-premultiplied color
	if (palfxMisc.w != 0.0) {
		c_straight = float3(1.0, 1.0, 1.0) - c_straight;
	}

	// Grayscale / Add / Mult
	c_straight = lerp(c_straight, float3((c_straight.r + c_straight.g + c_straight.b) / 3.0, (c_straight.r + c_straight.g + c_straight.b) / 3.0, (c_straight.r + c_straight.g + c_straight.b) / 3.0), palfxMisc.x) + add.rgb;
	c_straight *= mult.rgb;
	c_straight = clamp(c_straight, 0.0, 1.0);

	// Re-premultiply alpha
	FragColor.rgb = c_straight * alpha;
	return FragColor;
}
