<?php
// This file is generated. Do not modify it manually.
return array(
	'build' => array(
		'apiVersion' => 3,
		'name' => 'wp-3d-asset-editor/block',
		'title' => '3D Shared Scene',
		'category' => 'widgets',
		'icon' => 'media-code',
		'supports' => array(
			'html' => false
		),
		'attributes' => array(
			'sceneId' => array(
				'type' => 'number',
				'default' => 0
			)
		),
		'editorScript' => 'file:./index.js',
		'editorStyle' => 'file:./editor.css',
		'style' => 'file:./style.css'
	)
);
