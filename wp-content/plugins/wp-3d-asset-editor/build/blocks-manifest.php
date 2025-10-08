<?php
// This file is generated. Do not modify it manually.
return array(
	'build' => array(
		'apiVersion' => 3,
		'name' => 'wp-3d-asset-editor/block',
		'title' => '3D Shared Scene',
		'category' => 'widgets',
		'icon' => 'media',
		'supports' => array(
			'html' => false
		),
		'attributes' => array(
			'blockAssetId' => array(
				'type' => 'number',
				'default' => null
			),
			'assetUrl' => array(
				'type' => 'string',
				'default' => ''
			),
			'posX' => array(
				'type' => 'number',
				'default' => 0
			),
			'posY' => array(
				'type' => 'number',
				'default' => 0
			),
			'posZ' => array(
				'type' => 'number',
				'default' => 0
			),
			'rotX' => array(
				'type' => 'number',
				'default' => 0
			),
			'rotY' => array(
				'type' => 'number',
				'default' => 0
			),
			'rotZ' => array(
				'type' => 'number',
				'default' => 0
			)
		),
		'editorScript' => 'file:./index.js',
		'editorStyle' => 'file:./editor.css',
		'style' => 'file:./style.css'
	)
);
