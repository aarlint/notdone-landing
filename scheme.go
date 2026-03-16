package main

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

var metricsPodGroupVersion = schema.GroupVersion{Group: "metrics.k8s.io", Version: "v1beta1"}

var scheme = runtime.NewScheme()
var codecs = serializer.NewCodecFactory(scheme)
