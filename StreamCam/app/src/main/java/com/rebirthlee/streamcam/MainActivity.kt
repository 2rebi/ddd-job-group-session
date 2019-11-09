package com.rebirthlee.streamcam

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Bundle
import android.view.SurfaceHolder
import android.view.WindowManager
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.core.app.ActivityCompat
import androidx.core.content.ContextCompat
import com.pedro.rtplibrary.rtmp.RtmpCamera2
import kotlinx.android.synthetic.main.activity_main.*
import net.ossrs.rtmp.ConnectCheckerRtmp


private val tag = MainActivity::class.java.simpleName

class MainActivity : AppCompatActivity(), ConnectCheckerRtmp, SurfaceHolder.Callback {
    private val REQUEST_CODE_PERMISSIONS = 10
    private val REQUIRED_PERMISSIONS = arrayOf(Manifest.permission.CAMERA, Manifest.permission.RECORD_AUDIO)
    private lateinit var rtmpCamera2: RtmpCamera2
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        setContentView(R.layout.activity_main)

        rtmpCamera2 = RtmpCamera2(surfaceView, this)
        surfaceView.holder.addCallback(this)
        startStop.setOnClickListener {
            if (rtmpCamera2.isStreaming) {
                startStop.text = "시작"
                rtmpCamera2.stopStream()
            } else if (rtmpCamera2.prepareAudio() && rtmpCamera2.prepareVideo()) {
                startStop.text = "정지"
                rtmpCamera2.startStream(url.text?.toString() ?: "rtmp://172.30.1.60/live")
            }
        }

        if (!allPermissionsGranted()) {
            ActivityCompat.requestPermissions(
                this, REQUIRED_PERMISSIONS, REQUEST_CODE_PERMISSIONS)
        }

    }

    override fun onRequestPermissionsResult(
        requestCode: Int, permissions: Array<String>, grantResults: IntArray) {
        if (requestCode == REQUEST_CODE_PERMISSIONS) {
            if (allPermissionsGranted()) {
                surfaceView.post {
                    startActivity(Intent(this, MainActivity::class.java))
                    finish()
                }
            } else {
                Toast.makeText(this,
                    "권한을 허용해주세요.",
                    Toast.LENGTH_SHORT).show()
                finish()
            }
        }
    }

    private fun allPermissionsGranted() = REQUIRED_PERMISSIONS.all {
        ContextCompat.checkSelfPermission(
            baseContext, it) == PackageManager.PERMISSION_GRANTED
    }

    override fun onAuthSuccessRtmp() = runOnUiThread {
        Toast.makeText(
            this@MainActivity,
            "인증 성공.",
            Toast.LENGTH_SHORT
        ).show()
    }

    override fun onNewBitrateRtmp(bitrate: Long)  = runOnUiThread {
        Toast.makeText(
            this@MainActivity,
            "bitrate. $bitrate.",
            Toast.LENGTH_SHORT
        ).show()
    }

    override fun onConnectionSuccessRtmp() = runOnUiThread {
        Toast.makeText(
            this@MainActivity,
            "연결 성공.",
            Toast.LENGTH_SHORT
        ).show()
    }

    override fun onConnectionFailedRtmp(reason: String?) = runOnUiThread {
        Toast.makeText(
            this@MainActivity,
            "연결 실패. $reason",
            Toast.LENGTH_SHORT
        ).show()
        rtmpCamera2.stopStream()
    }

    override fun onAuthErrorRtmp() = runOnUiThread {
        Toast.makeText(
            this@MainActivity,
            "인증 실패.",
            Toast.LENGTH_SHORT
        ).show()
    }

    override fun onDisconnectRtmp() = runOnUiThread {
        Toast.makeText(
            this@MainActivity,
            "연결 끊김.",
            Toast.LENGTH_SHORT
        ).show()

        rtmpCamera2.startPreview()
    }


    override fun surfaceChanged(holder: SurfaceHolder?, format: Int, width: Int, height: Int) = rtmpCamera2.startPreview()
    override fun surfaceDestroyed(holder: SurfaceHolder?) {
        if (rtmpCamera2.isStreaming) {
            rtmpCamera2.stopStream()
        }
        rtmpCamera2.stopPreview()
    }

    override fun surfaceCreated(holder: SurfaceHolder?) = Unit
}
